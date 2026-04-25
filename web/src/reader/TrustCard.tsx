// TrustCard — one-line epistemic summary under the artifact title. Renders
// 3–5 secondary-tone badges so a reader can answer "can I trust this / why
// is it here / will the next agent see it" within three seconds. Missing
// axes gracefully fall back to a single "unclassified" chip so legacy rows
// (predating migration 0012 / artifact_meta) still read cleanly.
//
// Scope boundary (see Task reader-trust-card-...):
//   - Keep the colour palette SECONDARY so the existing draft/live/stale
//     lifecycle chip stays the primary visual anchor.
//   - No interactivity here — filtering by trust lives on the Sidebar
//     (separate Task). This component is display-only.

import type { ArtifactMeta, PinRef, RecentWarning, TaskMeta } from "../api/client";
import { useI18n } from "../i18n";

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
};

type ChipTone = "neutral" | "warning" | "danger";

type Chip = {
  label: string;
  tone: ChipTone;
  title: string;
};

type TFn = (key: string, ...args: Array<string | number>) => string;

export function TrustCard({ meta, pins, taskStatus, recentWarnings }: Props) {
  const { t } = useI18n();
  const chips = buildChips(meta, pins, t);
  const taskChip = taskStatusChip(taskStatus, t);
  if (taskChip) {
    chips.unshift(taskChip);
  }
  for (const warnChip of warningChips(recentWarnings, t)) {
    chips.push(warnChip);
  }
  if (chips.length === 0) {
    return null;
  }
  return (
    <div
      className="trust-card"
      aria-label="Trust summary"
      style={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: 6,
        margin: "4px 0 12px",
      }}
    >
      {chips.map((c) => (
        <span
          key={c.label}
          className={`chip chip--trust chip--trust-${c.tone}`}
          title={c.title}
          style={{
            fontSize: 10.5,
            letterSpacing: "0.02em",
            textTransform: "none",
            padding: "2px 7px",
            borderRadius: 999,
            border: "1px solid var(--fg-4)",
            background:
              c.tone === "danger"
                ? "color-mix(in oklch, var(--danger, #d86b6b) 14%, var(--bg-2))"
                : c.tone === "warning"
                ? "color-mix(in oklch, var(--warning, #c68a2a) 12%, var(--bg-2))"
                : "var(--bg-2)",
            color: "var(--fg-2)",
          }}
        >
          {c.label}
        </span>
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
  chips.push(sourceSummaryChip(meta, pins, t));

  // Next-session context policy — always relevant for retrieval reasoning.
  chips.push(nextSessionChip(meta, t));

  // Confidence — show low as a warning chip, hide medium/high to keep the
  // card quiet. Open question Q from the Task body: "low only" default.
  if (meta.confidence === "low") {
    chips.push({
      label: t("trust.confidence.low.label"),
      tone: "warning",
      title: t("trust.confidence.low.tip"),
    });
  }

  // Audience modifier — surfaced only when non-default (owner/approvers).
  if (meta.audience === "owner_only") {
    chips.push({
      label: t("trust.audience.owner_only.label"),
      tone: "danger",
      title: t("trust.audience.owner_only.tip"),
    });
  } else if (meta.audience === "approvers") {
    chips.push({
      label: t("trust.audience.approvers.label"),
      tone: "warning",
      title: t("trust.audience.approvers.tip"),
    });
  }

  return chips;
}

function trustClassChip(meta: ArtifactMeta, t: TFn): Chip {
  if (meta.source_type === "user_chat") {
    return {
      label: t("trust.source.user_chat.label"),
      tone: "warning",
      title: t("trust.source.user_chat.tip"),
    };
  }
  if (meta.verification_state === "verified") {
    return {
      label: t("trust.verification.verified.label"),
      tone: "neutral",
      title: t("trust.verification.verified.tip"),
    };
  }
  if (meta.verification_state === "unverified") {
    return {
      label: t("trust.verification.unverified.label"),
      tone: "warning",
      title: t("trust.verification.unverified.tip"),
    };
  }
  return {
    label: t("trust.verification.partially_verified.label"),
    tone: "neutral",
    title: t("trust.verification.partially_verified.tip"),
  };
}

function sourceSummaryChip(meta: ArtifactMeta, pins: PinRef[] | undefined, t: TFn): Chip {
  const count = pins?.length ?? 0;
  const pinsLabel = pinCountLabel(count, t);
  if (meta.source_type === "code") {
    return {
      label: count > 0 ? `${t("trust.source.code.label")} · ${pinsLabel}` : t("trust.source.code.label"),
      tone: "neutral",
      title: t("trust.source.code.tip"),
    };
  }
  if (meta.source_type === "mixed") {
    return {
      label: count > 0 ? `${t("trust.source.mixed.label")} · ${pinsLabel}` : t("trust.source.mixed.label"),
      tone: "neutral",
      title: t("trust.source.mixed.tip"),
    };
  }
  if (meta.source_type === "artifact") {
    return {
      label: t("trust.source.artifact.label"),
      tone: "neutral",
      title: t("trust.source.artifact.tip"),
    };
  }
  if (meta.source_type === "external") {
    return {
      label: t("trust.source.external.label"),
      tone: "neutral",
      title: t("trust.source.external.tip"),
    };
  }
  if (meta.source_type === "user_chat") {
    return {
      label: t("trust.source.user_chat.short_label"),
      tone: "warning",
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
      return {
        label: t("trust.task.claimed_done.label"),
        tone: "warning",
        title: t("trust.task.claimed_done.tip"),
      };
    case "verified":
      return {
        label: t("trust.task.verified.label"),
        tone: "neutral",
        title: t("trust.task.verified.tip"),
      };
    case "blocked":
      return {
        label: t("trust.task.blocked.label"),
        tone: "danger",
        title: t("trust.task.blocked.tip"),
      };
    case "cancelled":
      return {
        label: t("trust.task.cancelled.label"),
        tone: "neutral",
        title: t("trust.task.cancelled.tip"),
      };
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
    chips.push({
      label: t("trust.warning.canonical_rewrite.label"),
      tone: "warning",
      title: t("trust.warning.canonical_rewrite.tip"),
    });
  }
  if (codes.has("CONSENT_REQUIRED_FOR_USER_CHAT")) {
    chips.push({
      label: t("trust.warning.consent_required.label"),
      tone: "warning",
      title: t("trust.warning.consent_required.tip"),
    });
  }
  if (codes.has("SOURCE_TYPE_UNCLASSIFIED")) {
    chips.push({
      label: t("trust.warning.source_unclassified.label"),
      tone: "neutral",
      title: t("trust.warning.source_unclassified.tip"),
    });
  }
  if (codes.has("RECOMMEND_READ_BEFORE_CREATE")) {
    chips.push({
      label: t("trust.warning.near_duplicate.label"),
      tone: "neutral",
      title: t("trust.warning.near_duplicate.tip"),
    });
  }
  return chips;
}

function nextSessionChip(meta: ArtifactMeta, t: TFn): Chip {
  switch (meta.next_context_policy) {
    case "excluded":
      return {
        label: t("trust.next_context.excluded.label"),
        tone: "danger",
        title: t("trust.next_context.excluded.tip"),
      };
    case "opt_in":
      return {
        label: t("trust.next_context.opt_in.label"),
        tone: "warning",
        title: t("trust.next_context.opt_in.tip"),
      };
    case "default":
      return {
        label: t("trust.next_context.default.label"),
        tone: "neutral",
        title: t("trust.next_context.default.tip"),
      };
    default:
      return {
        label: t("trust.next_context.default.label"),
        tone: "neutral",
        title: t("trust.next_context.default_missing.tip"),
      };
  }
}
