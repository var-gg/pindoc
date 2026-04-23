import type { ArtifactRef } from "../api/client";
import { agentAvatar } from "./avatars";
import { useI18n } from "../i18n";

// ArtifactByline renders the dual-identity header line for an artifact:
// "<user_avatar> Jaime · via claude-code (opus-4.7)". The user is the
// human who owns the MCP session (users.display_name via
// artifacts.author_user_id); the agent is the string author_id label
// the client reported on propose.
//
// Fallback: artifacts predating migration 0014 (and installs where the
// operator skipped PINDOC_USER_NAME) have author_user=null. In that
// case we show "(unknown)" in the user slot so the agent identity
// stays legible and the gap is obvious.

type Props = {
  artifact: Pick<ArtifactRef, "author_id" | "author_user"> & { author_version?: string };
  // variant controls density; "inline" is the one-line version used on
  // the Reader detail header, "list" is the compact version used in
  // backlink rows where horizontal room is scarce.
  variant?: "inline" | "list";
};

export function ArtifactByline({ artifact, variant = "inline" }: Props) {
  const { t } = useI18n();
  const display = artifact.author_user?.display_name ?? t("reader.byline_unknown");
  const av = agentAvatar(artifact.author_user?.github_handle ?? display);
  const agent = artifact.author_version
    ? `${artifact.author_id} (${artifact.author_version})`
    : artifact.author_id;

  if (variant === "list") {
    return (
      <span className="byline byline--list">
        <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>
          {av.initials}
        </span>
        <span>{display}</span>
        <span className="byline__sep">·</span>
        <span className="byline__agent">{agent}</span>
      </span>
    );
  }

  return (
    <span className="byline">
      <span className={av.className} style={{ width: 18, height: 18, fontSize: 9 }}>
        {av.initials}
      </span>
      <span className="byline__name">{display}</span>
      <span className="byline__sep">·</span>
      <span className="byline__agent">
        {t("reader.byline_via", agent)}
      </span>
    </span>
  );
}
