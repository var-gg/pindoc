// TrustCard — one-line epistemic summary under the artifact title. Renders
// 3–5 secondary-tone badges so a reader can answer "can I trust this / why
// is it here / will the next agent see it" within three seconds. Missing
// axes gracefully fall back to a single "unclassified" chip so legacy rows
// (predating migration 0012 / artifact_meta) still read cleanly.

import type { ArtifactMeta, ArtifactReadState, PinRef, RecentWarning, TaskMeta } from "../api/client";
import { useI18n } from "../i18n";
import { BadgePopoverChip } from "./BadgePopoverChip";
import type { BadgeFilter } from "./badgeFilters";

type Props = {
  meta?: ArtifactMeta;
  pins?: PinRef[];
  // taskStatus surfaces the Task lifecycle v2 state (migration 0013) as
  // a dedicated chip: claimed_done shows as "unverified work" tone, and
  // verified shows as "agent-verified" neutral tone. Non-Task artifacts
  // pass undefined and the chip is skipped.
  taskStatus?: TaskMeta["status"];
  // recentWarnings are events.artifact.warning_raised rows served by
  // /api/p/:project/artifacts/:slug (Task propose-경로-warning-영속화).
  // Only codes from the latest revision render as chips — older-
  // revision codes stay in the history but don't clutter the card for
  // an artifact whose current revision is clean.
  recentWarnings?: RecentWarning[];
  // readState is the Layer 2 signal — has a human actually read this?
  // Layer 4 verification stays separate; the chip is the bridge between
  // them on the card. 'deeply_read' nudges agents that this artifact is
  // ready for explicit verify; 'unseen' / 'glanced' warns it is not.
  readState?: ArtifactReadState;
  onApplyFilter?: (filter: BadgeFilter) => void;
  legendHref?: string;
};

type ChipTone = "neutral" | "warning" | "danger";

type Chip = {
  label: string;
  tone: ChipTone;
  title: string;
  filter?: BadgeFilter;
};

type TFn = (key: string, ...args: Array<string | number>) => string;

export function TrustCard({
  meta,
  pins,
  taskStatus,
  recentWarnings,
  readState,
  onApplyFilter,
  legendHref,
}: Props) {
  const { t } = useI18n();
  const chips = buildChips(meta, pins, t);
  const taskChip = taskStatusChip(taskStatus, t);
  if (taskChip) {
    chips.unshift(taskChip);
  }
  const readChip = readStateChip(readState, t);
  if (readChip) {
    chips.push(readChip);
  }
  for (const warnChip of warningChips(recentWarnings, t)) {
    chips.push(warnChip);
  }
  if (chips.length === 0) {
    return null;
  }
  return (
    <div className="trust-card" aria-label="Trust summary">
      {chips.map((c, i) => (
        <BadgePopoverChip
          key={`${c.label}-${i}`}
          label={c.label}
          description={c.title}
          className={`chip chip--trust chip--trust-${c.tone}`}
          onApply={c.filter && onApplyFilter ? () => onApplyFilter(c.filter!) : undefined}
          legendHref={legendHref}
        />
      ))}
    </div>
  );
}

function buildChips(meta: ArtifactMeta | undefined, pins: PinRef[] | undefined, t: TFn): Chip[] {
  const chips: Chip[] = [];
  if (!meta || Object.keys(meta).length === 0) {
    chips.push({
      label: t("trust.unclassified.label"),
      tone: "neutral",
      title: t("trust.unclassified.tip"),
    });
    return chips;
  }

  // Trust class — composite of verification_state + source_type + audience.
  // One chip, highest-signal value wins: private > stale-like > verified etc.
  chips.push(trustClassChip(meta, t));

  // Source summary — combines source_type with pins count.
  const sourceChip = sourceSummaryChip(meta, pins, t);
  if (sourceChip) chips.push(sourceChip);

  // Next-session context policy — always relevant for retrieval reasoning.
  chips.push(nextSessionChip(meta, t));

  // Confidence — show low as a warning chip, hide medium/high to keep the
  // card quiet. Open question Q from the Task body: "low only" default.
  if (meta.confidence === "low") {
    const label = t("trust.confidence.low.label");
    chips.push({
      label,
      tone: "warning",
      title: t("trust.confidence.low.tip"),
      filter: { key: "confidence", value: "low", label },
    });
  }

  // Audience modifier — surfaced only when non-default (owner/approvers).
  if (meta.audience === "owner_only") {
    const label = t("trust.audience.owner_only.label");
    chips.push({
      label,
      tone: "danger",
      title: t("trust.audience.owner_only.tip"),
      filter: { key: "audience", value: "owner_only", label },
    });
  } else if (meta.audience === "approvers") {
    const label = t("trust.audience.approvers.label");
    chips.push({
      label,
      tone: "warning",
      title: t("trust.audience.approvers.tip"),
      filter: { key: "audience", value: "approvers", label },
    });
  }

  return chips;
}

function readStateChip(state: ArtifactReadState | undefined, t: TFn): Chip | null {
  if (!state || state.read_state === "unseen") return null;
  const pct = Math.round((state.completion_pct ?? 0) * 100);
  switch (state.read_state) {
    case "deeply_read":
      return {
        label: t("trust.read_state.deeply_read.label"),
        tone: "neutral",
        title: t("trust.read_state.deeply_read.tip", pct),
      };
    case "read":
      return {
        label: t("trust.read_state.read.label"),
        tone: "neutral",
        title: t("trust.read_state.read.tip", pct),
      };
    case "glanced":
      return {
        label: t("trust.read_state.glanced.label"),
        tone: "warning",
        title: t("trust.read_state.glanced.tip", pct),
      };
    default:
      return null;
  }
}

function trustClassChip(meta: ArtifactMeta, t: TFn): Chip {
  if (meta.source_type === "user_chat") {
    const label = t("trust.source.user_chat.label");
    return {
      label,
      tone: "warning",
      title: t("trust.source.user_chat.tip"),
      filter: { key: "source_type", value: "user_chat", label },
    };
  }
  if (meta.verification_state === "verified") {
    const label = t("trust.verification.verified.label");
    return {
      label,
      tone: "neutral",
      title: t("trust.verification.verified.tip"),
      filter: { key: "verification_state", value: "verified", label },
    };
  }
  if (meta.verification_state === "unverified") {
    const label = t("trust.verification.unverified.label");
    return {
      label,
      tone: "warning",
      title: t("trust.verification.unverified.tip"),
      filter: { key: "verification_state", value: "unverified", label },
    };
  }
  const label = t("trust.verification.partially_verified.label");
  return {
    label,
    tone: "neutral",
    title: t("trust.verification.partially_verified.tip"),
    filter: { key: "verification_state", value: "partially_verified", label },
  };
}

function sourceSummaryChip(meta: ArtifactMeta, pins: PinRef[] | undefined, t: TFn): Chip | null {
  const count = pins?.length ?? 0;
  const pinsLabel = pinCountLabel(count, t);
  if (meta.source_type === "code") {
    const label = t("trust.source.code.label");
    return {
      label: count > 0 ? `${label} · ${pinsLabel}` : label,
      tone: "neutral",
      title: t("trust.source.code.tip"),
      filter: { key: "source_type", value: "code", label },
    };
  }
  if (meta.source_type === "mixed") {
    const label = t("trust.source.mixed.label");
    return {
      label: count > 0 ? `${label} · ${pinsLabel}` : label,
      tone: "neutral",
      title: t("trust.source.mixed.tip"),
      filter: { key: "source_type", value: "mixed", label },
    };
  }
  if (meta.source_type === "artifact") {
    const label = t("trust.source.artifact.label");
    return {
      label,
      tone: "neutral",
      title: t("trust.source.artifact.tip"),
      filter: { key: "source_type", value: "artifact", label },
    };
  }
  if (meta.source_type === "external") {
    const label = t("trust.source.external.label");
    return {
      label,
      tone: "neutral",
      title: t("trust.source.external.tip"),
      filter: { key: "source_type", value: "external", label },
    };
  }
  if (meta.source_type === "user_chat") {
    if (count === 0) return null;
    return {
      label: pinsLabel,
      tone: "neutral",
      title: t("trust.source.user_chat.tip"),
    };
  }
  return {
    label: count > 0 ? pinsLabel : t("trust.pins_none"),
    tone: "neutral",
    title: t("trust.source.unclassified.tip"),
  };
}

function pinCountLabel(count: number, t: TFn): string {
  return count === 1 ? t("trust.pins_one") : t("trust.pins_many", count);
}

function taskStatusChip(status: TaskMeta["status"], t: TFn): Chip | null {
  switch (status) {
    case "claimed_done":
      {
        const label = t("trust.task.claimed_done.label");
        return {
          label,
          tone: "warning",
          title: t("trust.task.claimed_done.tip"),
          filter: { key: "task_status", value: "claimed_done", label },
        };
      }
    case "verified":
      {
        const label = t("trust.task.verified.label");
        return {
          label,
          tone: "neutral",
          title: t("trust.task.verified.tip"),
          filter: { key: "task_status", value: "verified", label },
        };
      }
    case "blocked":
      {
        const label = t("trust.task.blocked.label");
        return {
          label,
          tone: "danger",
          title: t("trust.task.blocked.tip"),
          filter: { key: "task_status", value: "blocked", label },
        };
      }
    case "cancelled":
      {
        const label = t("trust.task.cancelled.label");
        return {
          label,
          tone: "neutral",
          title: t("trust.task.cancelled.tip"),
          filter: { key: "task_status", value: "cancelled", label },
        };
      }
    case "open":
    case undefined:
    default:
      return null;
  }
}

// warningChips maps events.artifact.warning_raised codes onto Trust Card
// chips. We only render codes from the latest revision so the card
// reflects the artifact's *current* state — an old revision that raised
// a warning now resolved in the head revision shouldn't keep spooking
// readers. Table kept narrow: unknown codes are skipped rather than
// producing a cryptic "code: FOO" chip. Task propose-경로-warning-
// 영속화 §Trust Card 매핑.
function warningChips(recent: RecentWarning[] | undefined, t: TFn): Chip[] {
  if (!recent || recent.length === 0) return [];
  // Find the max revision_number among delivered events. The server
  // already orders by created_at DESC and caps at 5, which usually
  // means the first row *is* the latest revision's — but an older-
  // revision event inserted after a newer one would break that
  // assumption, so we re-derive the max explicitly.
  let latest = 0;
  for (const w of recent) {
    if (w.revision_number > latest) latest = w.revision_number;
  }
  const codes = new Set<string>();
  for (const w of recent) {
    if (w.revision_number !== latest) continue;
    for (const c of w.codes) codes.add(c);
  }
  const chips: Chip[] = [];
  if (codes.has("CANONICAL_REWRITE_WITHOUT_EVIDENCE")) {
    const label = t("trust.warning.canonical_rewrite.label");
    chips.push({
      label,
      tone: "warning",
      title: t("trust.warning.canonical_rewrite.tip"),
      filter: { key: "warning", value: "CANONICAL_REWRITE_WITHOUT_EVIDENCE", label },
    });
  }
  if (codes.has("CONSENT_REQUIRED_FOR_USER_CHAT")) {
    const label = t("trust.warning.consent_required.label");
    chips.push({
      label,
      tone: "warning",
      title: t("trust.warning.consent_required.tip"),
      filter: { key: "warning", value: "CONSENT_REQUIRED_FOR_USER_CHAT", label },
    });
  }
  if (codes.has("SOURCE_TYPE_UNCLASSIFIED")) {
    const label = t("trust.warning.source_unclassified.label");
    chips.push({
      label,
      tone: "neutral",
      title: t("trust.warning.source_unclassified.tip"),
      filter: { key: "warning", value: "SOURCE_TYPE_UNCLASSIFIED", label },
    });
  }
  if (codes.has("RECOMMEND_READ_BEFORE_CREATE")) {
    const label = t("trust.warning.near_duplicate.label");
    chips.push({
      label,
      tone: "neutral",
      title: t("trust.warning.near_duplicate.tip"),
      filter: { key: "warning", value: "RECOMMEND_READ_BEFORE_CREATE", label },
    });
  }
  return chips;
}

function nextSessionChip(meta: ArtifactMeta, t: TFn): Chip {
  switch (meta.next_context_policy) {
    case "excluded":
      {
        const label = t("trust.next_context.excluded.label");
        return {
          label,
          tone: "danger",
          title: t("trust.next_context.excluded.tip"),
          filter: { key: "next_context_policy", value: "excluded", label },
        };
      }
    case "opt_in":
      {
        const label = t("trust.next_context.opt_in.label");
        return {
          label,
          tone: "warning",
          title: t("trust.next_context.opt_in.tip"),
          filter: { key: "next_context_policy", value: "opt_in", label },
        };
      }
    case "default":
      {
        const label = t("trust.next_context.default.label");
        return {
          label,
          tone: "neutral",
          title: t("trust.next_context.default.tip"),
          filter: { key: "next_context_policy", value: "default", label },
        };
      }
    default:
      {
        const label = t("trust.next_context.default.label");
        return {
          label,
          tone: "neutral",
          title: t("trust.next_context.default_missing.tip"),
          filter: { key: "next_context_policy", value: "default", label },
        };
      }
  }
}
