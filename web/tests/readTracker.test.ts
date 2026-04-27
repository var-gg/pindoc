import { createReadTracker, ReadTrackerCore, type ReadTrackerFlushReason } from "../src/reader/readTracker";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function newCore() {
  let now = 0;
  return {
    core: new ReadTrackerCore({ nowMs: () => now, idleAfterMs: 5_000 }),
    advance(ms: number) {
      now += ms;
      return now;
    },
  };
}

function testAccumulatesOnlyWhenAllAxesAreTrue(): void {
  const t = newCore();
  t.core.setVisible(true);
  t.core.setIntersecting(true);
  t.core.notifyActivity();
  t.advance(1_000);
  assertEqual(t.core.tick().activeSeconds, 1, "all axes true should count active time");
}

function testVisibilityFalseStopsActiveTime(): void {
  const t = newCore();
  t.core.setVisible(false);
  t.core.setIntersecting(true);
  t.core.notifyActivity();
  t.advance(1_000);
  assertEqual(t.core.tick().activeSeconds, 0, "hidden document should not count active time");
}

function testIntersectionFalseStopsActiveTime(): void {
  const t = newCore();
  t.core.setVisible(true);
  t.core.setIntersecting(false);
  t.core.notifyActivity();
  t.advance(1_000);
  assertEqual(t.core.tick().activeSeconds, 0, "non-intersecting body should not count active time");
}

function testIdleFalseAxisStopsActiveTimeAfterThreshold(): void {
  const t = newCore();
  t.core.setVisible(true);
  t.core.setIntersecting(true);
  t.core.notifyActivity();
  t.advance(6_000);
  const snapshot = t.core.tick();
  assertEqual(snapshot.activeSeconds, 5, "active time should stop at idle threshold");
  assertEqual(snapshot.idleSeconds, 1, "idle time should count after threshold");
  assert(snapshot.idle, "snapshot should report idle");
}

function testFlushReasons(): void {
  const realDateNow = Date.now;
  let now = 0;
  Date.now = () => now;
  try {
    for (const reason of ["hidden", "route", "beforeunload"] as ReadTrackerFlushReason[]) {
      const fake = fakeDOM();
      const reasons: string[] = [];
      const tracker = createReadTracker({
        artifactId: "artifact-1",
        bodyElement: fake.element,
        flush: (_payload, flushReason) => {
          reasons.push(flushReason);
        },
      });
      now += 1_000;
      if (reason === "hidden") {
        fake.document.visibilityState = "hidden";
        fake.document.emit("visibilitychange");
      } else if (reason === "beforeunload") {
        fake.window.emit("beforeunload");
      } else {
        tracker.stop("route");
      }
      assert(reasons.includes(reason), `${reason} should flush read event`);
      tracker.stop("route");
    }
  } finally {
    Date.now = realDateNow;
  }
}

function fakeDOM(): { window: FakeRuntimeTarget; document: FakeRuntimeDocument; element: HTMLElement } {
  const win = new FakeEventTarget() as unknown as FakeRuntimeTarget & {
    innerHeight: number;
    setInterval: Window["setInterval"];
    clearInterval: Window["clearInterval"];
  };
  win.innerHeight = 600;
  win.setInterval = () => 1;
  win.clearInterval = () => undefined;
  const doc = new FakeEventTarget() as unknown as FakeRuntimeDocument & {
    defaultView: Window;
    documentElement: HTMLElement;
  };
  doc.defaultView = win as unknown as Window;
  doc.visibilityState = "visible";
  doc.documentElement = { clientHeight: 600 } as HTMLElement;
  const element = {
    ownerDocument: doc,
    scrollHeight: 1_000,
    getBoundingClientRect: () => ({ top: 0, height: 1_000 }),
  } as unknown as HTMLElement;
  return { window: win, document: doc, element };
}

class FakeEventTarget {
  private listeners = new Map<string, Array<() => void>>();

  addEventListener(name: string, listener: () => void): void {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener]);
  }

  removeEventListener(name: string, listener: () => void): void {
    this.listeners.set(name, (this.listeners.get(name) ?? []).filter((candidate) => candidate !== listener));
  }

  emit(name: string): void {
    for (const listener of this.listeners.get(name) ?? []) listener();
  }
}

type FakeRuntimeTarget = {
  emit: (name: string) => void;
};

type FakeRuntimeDocument = FakeRuntimeTarget & {
  visibilityState: DocumentVisibilityState;
};

testAccumulatesOnlyWhenAllAxesAreTrue();
testVisibilityFalseStopsActiveTime();
testIntersectionFalseStopsActiveTime();
testIdleFalseAxisStopsActiveTimeAfterThreshold();
testFlushReasons();
