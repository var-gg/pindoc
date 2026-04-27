import type { ReadEventInput } from "../api/client";

export type ReadTrackerSnapshot = {
  startedAtMs: number;
  endedAtMs: number;
  activeSeconds: number;
  idleSeconds: number;
  scrollMaxPct: number;
  visible: boolean;
  intersecting: boolean;
  idle: boolean;
};

export type ReadTrackerFlushReason = "hidden" | "route" | "beforeunload";

export type ReadTrackerOptions = {
  artifactId: string;
  locale?: string;
  bodyElement: HTMLElement;
  idleAfterMs?: number;
  onUpdate?: (snapshot: ReadTrackerSnapshot) => void;
  flush: (payload: ReadEventInput, reason: ReadTrackerFlushReason) => void | Promise<void>;
};

export class ReadTrackerCore {
  private readonly nowMs: () => number;
  private readonly idleAfterMs: number;
  private startedAtMs: number;
  private lastTickMs: number;
  private lastActivityMs: number;
  private activeMs = 0;
  private idleMs = 0;
  private visible = true;
  private intersecting = false;
  private idle = false;
  private scrollMaxPct = 0;

  constructor(opts: { nowMs?: () => number; idleAfterMs?: number } = {}) {
    this.nowMs = opts.nowMs ?? (() => Date.now());
    this.idleAfterMs = opts.idleAfterMs ?? 45_000;
    const now = this.nowMs();
    this.startedAtMs = now;
    this.lastTickMs = now;
    this.lastActivityMs = now;
  }

  setVisible(next: boolean): void {
    this.advance();
    this.visible = next;
  }

  setIntersecting(next: boolean): void {
    this.advance();
    this.intersecting = next;
  }

  notifyActivity(): void {
    this.advance();
    this.lastActivityMs = this.nowMs();
    this.idle = false;
  }

  recordScroll(percent: number): void {
    const clamped = Math.max(0, Math.min(1, percent));
    if (Number.isFinite(clamped)) {
      this.scrollMaxPct = Math.max(this.scrollMaxPct, clamped);
    }
  }

  tick(): ReadTrackerSnapshot {
    this.advance();
    return this.snapshot();
  }

  flushSnapshot(): ReadTrackerSnapshot {
    this.advance();
    return this.snapshot();
  }

  reset(): void {
    const now = this.nowMs();
    this.startedAtMs = now;
    this.lastTickMs = now;
    this.lastActivityMs = now;
    this.activeMs = 0;
    this.idleMs = 0;
    this.idle = false;
    this.scrollMaxPct = 0;
  }

  snapshot(): ReadTrackerSnapshot {
    return {
      startedAtMs: this.startedAtMs,
      endedAtMs: this.lastTickMs,
      activeSeconds: roundSeconds(this.activeMs),
      idleSeconds: roundSeconds(this.idleMs),
      scrollMaxPct: this.scrollMaxPct,
      visible: this.visible,
      intersecting: this.intersecting,
      idle: this.idle,
    };
  }

  private advance(): void {
    const now = this.nowMs();
    if (now < this.lastTickMs) {
      this.lastTickMs = now;
      return;
    }
    const delta = now - this.lastTickMs;
    if (delta > 0 && this.visible && this.intersecting) {
      const idleBoundary = this.lastActivityMs + this.idleAfterMs;
      if (this.idle || this.lastTickMs >= idleBoundary) {
        this.idleMs += delta;
      } else if (now <= idleBoundary) {
        this.activeMs += delta;
      } else {
        this.activeMs += idleBoundary - this.lastTickMs;
        this.idleMs += now - idleBoundary;
      }
    }
    this.idle = now - this.lastActivityMs >= this.idleAfterMs;
    this.lastTickMs = now;
  }
}

export function createReadTracker(opts: ReadTrackerOptions): { stop: (reason?: ReadTrackerFlushReason) => void; snapshot: () => ReadTrackerSnapshot } {
  const core = new ReadTrackerCore({ idleAfterMs: opts.idleAfterMs });
  let stopped = false;
  const doc = opts.bodyElement.ownerDocument;
  const win = doc.defaultView;
  const update = () => opts.onUpdate?.(core.tick());
  const flush = (reason: ReadTrackerFlushReason) => {
    const snapshot = core.flushSnapshot();
    if (snapshot.endedAtMs <= snapshot.startedAtMs) return;
    if (snapshot.activeSeconds <= 0 && snapshot.scrollMaxPct <= 0) return;
    const payload: ReadEventInput = {
      artifact_id: opts.artifactId,
      started_at: new Date(snapshot.startedAtMs).toISOString(),
      ended_at: new Date(snapshot.endedAtMs).toISOString(),
      active_seconds: snapshot.activeSeconds,
      scroll_max_pct: snapshot.scrollMaxPct,
      idle_seconds: snapshot.idleSeconds,
      locale: opts.locale,
    };
    void opts.flush(payload, reason);
  };

  const recordBodyProgress = () => {
    core.recordScroll(bodyProgress(opts.bodyElement, win));
    core.notifyActivity();
    update();
  };
  const markActivity = () => {
    core.notifyActivity();
    update();
  };
  const handleVisibility = () => {
    const visible = doc.visibilityState !== "hidden";
    core.setVisible(visible);
    if (!visible) {
      flush("hidden");
      core.reset();
    }
    update();
  };
  const handleBeforeUnload = () => {
    flush("beforeunload");
  };

  core.setVisible(doc.visibilityState !== "hidden");
  core.recordScroll(bodyProgress(opts.bodyElement, win));

  let observer: IntersectionObserver | null = null;
  if (win && "IntersectionObserver" in win) {
    observer = new win.IntersectionObserver((entries) => {
      core.setIntersecting(entries.some((entry) => entry.isIntersecting));
      update();
    }, { threshold: [0, 0.1, 0.35, 0.75, 1] });
    observer.observe(opts.bodyElement);
  } else {
    core.setIntersecting(true);
  }

  const activityEvents = ["mousemove", "pointerdown", "keydown", "touchstart", "wheel"] as const;
  for (const eventName of activityEvents) {
    win?.addEventListener(eventName, markActivity, { passive: true });
  }
  win?.addEventListener("scroll", recordBodyProgress, { passive: true });
  win?.addEventListener("resize", recordBodyProgress, { passive: true });
  doc.addEventListener("visibilitychange", handleVisibility);
  win?.addEventListener("beforeunload", handleBeforeUnload);
  const interval = win?.setInterval(update, 1_000);

  update();

  const cleanup = () => {
    observer?.disconnect();
    for (const eventName of activityEvents) {
      win?.removeEventListener(eventName, markActivity);
    }
    win?.removeEventListener("scroll", recordBodyProgress);
    win?.removeEventListener("resize", recordBodyProgress);
    doc.removeEventListener("visibilitychange", handleVisibility);
    win?.removeEventListener("beforeunload", handleBeforeUnload);
    if (interval !== undefined) {
      win?.clearInterval(interval);
    }
  };

  return {
    stop(reason = "route") {
      if (stopped) return;
      stopped = true;
      flush(reason);
      cleanup();
    },
    snapshot() {
      return core.snapshot();
    },
  };
}

function bodyProgress(element: HTMLElement, win: Window | null): number {
  if (!win) return 0;
  const rect = element.getBoundingClientRect();
  const viewportHeight = win.innerHeight || element.ownerDocument.documentElement.clientHeight || 0;
  const totalHeight = Math.max(element.scrollHeight, rect.height, 1);
  const viewedBottom = Math.min(totalHeight, Math.max(0, viewportHeight - rect.top));
  return viewedBottom / totalHeight;
}

function roundSeconds(ms: number): number {
  return Math.floor(ms / 100) / 10;
}
