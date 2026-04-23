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

export function TrustCard({ meta, pins, taskStatus, recentWarnings }: Props) {
  const chips = buildChips(meta, pins);
  const taskChip = taskStatusChip(taskStatus);
  if (taskChip) {
    chips.unshift(taskChip);
  }
  for (const warnChip of warningChips(recentWarnings)) {
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

function buildChips(meta: ArtifactMeta | undefined, pins: PinRef[] | undefined): Chip[] {
  const chips: Chip[] = [];
  if (!meta || Object.keys(meta).length === 0) {
    chips.push({
      label: "Unclassified",
      tone: "neutral",
      title: "This artifact has no epistemic metadata. Treat as unverified context.",
    });
    return chips;
  }

  // Trust class — composite of verification_state + source_type + audience.
  // One chip, highest-signal value wins: private > stale-like > verified etc.
  chips.push(trustClassChip(meta));

  // Source summary — combines source_type with pins count.
  chips.push(sourceSummaryChip(meta, pins));

  // Next-session context policy — always relevant for retrieval reasoning.
  chips.push(nextSessionChip(meta));

  // Confidence — show low as a warning chip, hide medium/high to keep the
  // card quiet. Open question Q from the Task body: "low only" default.
  if (meta.confidence === "low") {
    chips.push({
      label: "Confidence: low",
      tone: "warning",
      title: "Agent self-reported low confidence. Verify before reusing as authority.",
    });
  }

  // Audience modifier — surfaced only when non-default (owner/approvers).
  if (meta.audience === "owner_only") {
    chips.push({
      label: "Owner only",
      tone: "danger",
      title: "Narrow audience — do not share outside the artifact owner.",
    });
  } else if (meta.audience === "approvers") {
    chips.push({
      label: "Approvers only",
      tone: "warning",
      title: "Limited audience — approvers-tier readers.",
    });
  }

  return chips;
}

function trustClassChip(meta: ArtifactMeta): Chip {
  if (meta.source_type === "user_chat") {
    return {
      label: "Conversation-derived",
      tone: "warning",
      title: "Derived from user-chat substrate. Canonicalisation needs explicit consent.",
    };
  }
  if (meta.verification_state === "verified") {
    return {
      label: "Verified",
      tone: "neutral",
      title: "Explicitly marked verified against code or authoritative evidence.",
    };
  }
  if (meta.verification_state === "unverified") {
    return {
      label: "Unverified",
      tone: "warning",
      title: "No evidence signal attached. Treat as hypothesis until confirmed.",
    };
  }
  return {
    label: "Partially verified",
    tone: "neutral",
    title: "Some evidence (pins or partial verification). Read before reusing.",
  };
}

function sourceSummaryChip(meta: ArtifactMeta, pins: PinRef[] | undefined): Chip {
  const count = pins?.length ?? 0;
  if (meta.source_type === "code") {
    return {
      label: count > 0 ? `Code · ${count} pin${count === 1 ? "" : "s"}` : "Code substrate",
      tone: "neutral",
      title: "Grounded in repository code pins.",
    };
  }
  if (meta.source_type === "mixed") {
    return {
      label: count > 0 ? `Mixed · ${count} pin${count === 1 ? "" : "s"}` : "Mixed substrate",
      tone: "neutral",
      title: "Combines code, chat, and/or external sources.",
    };
  }
  if (meta.source_type === "artifact") {
    return {
      label: "Artifact-derived",
      tone: "neutral",
      title: "Derived from other artifacts (see relates_to).",
    };
  }
  if (meta.source_type === "external") {
    return {
      label: "External",
      tone: "neutral",
      title: "Derived from external references (documentation, specs, URLs).",
    };
  }
  if (meta.source_type === "user_chat") {
    return {
      label: "User chat",
      tone: "warning",
      title: "Derived from a user conversation turn.",
    };
  }
  return {
    label: count > 0 ? `${count} pin${count === 1 ? "" : "s"}` : "No pins",
    tone: "neutral",
    title: "Source type not classified.",
  };
}

function taskStatusChip(status: TaskMeta["status"]): Chip | null {
  switch (status) {
    case "claimed_done":
      return {
        label: "Claimed done — unverified",
        tone: "warning",
        title:
          "Implementing agent self-attested completion. No verifier agent has filed a VerificationReport yet.",
      };
    case "verified":
      return {
        label: "Agent-verified",
        tone: "neutral",
        title:
          "A different agent filed a VerificationReport via pindoc.artifact.verify.",
      };
    case "blocked":
      return {
        label: "Blocked",
        tone: "danger",
        title: "External dependency blocks progress.",
      };
    case "cancelled":
      return {
        label: "Cancelled",
        tone: "neutral",
        title: "Task was abandoned.",
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
function warningChips(recent: RecentWarning[] | undefined): Chip[] {
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
      label: "Uncertain rewrite",
      tone: "warning",
      title:
        "Canonical section changed on this revision without new evidence (pins, verification bump, or evidence-keyword commit_msg).",
    });
  }
  if (codes.has("CONSENT_REQUIRED_FOR_USER_CHAT")) {
    chips.push({
      label: "Consent pending",
      tone: "warning",
      title:
        "source_type=user_chat without explicit consent_state. Author needs to classify before canonicalising.",
    });
  }
  if (codes.has("SOURCE_TYPE_UNCLASSIFIED")) {
    chips.push({
      label: "Unclassified source",
      tone: "neutral",
      title: "No code pins and user-chat quote pattern detected — classify source_type on next revision.",
    });
  }
  if (codes.has("RECOMMEND_READ_BEFORE_CREATE")) {
    chips.push({
      label: "Near-duplicate candidate",
      tone: "neutral",
      title:
        "Semantic search found a near-neighbour (distance 0.18-0.25). Consider update_of on the existing artifact.",
    });
  }
  return chips;
}

function nextSessionChip(meta: ArtifactMeta): Chip {
  switch (meta.next_context_policy) {
    case "excluded":
      return {
        label: "Excluded from context",
        tone: "danger",
        title: "Server skips this artifact when building next-session context.",
      };
    case "opt_in":
      return {
        label: "Context: opt-in",
        tone: "warning",
        title: "Surfaces only when explicitly queried, not in default Fast Landing.",
      };
    case "default":
      return {
        label: "Context: default",
        tone: "neutral",
        title: "Eligible for default next-session Fast Landing.",
      };
    default:
      return {
        label: "Context: default",
        tone: "neutral",
        title: "No explicit policy. Server treats as default-eligible.",
      };
  }
}
