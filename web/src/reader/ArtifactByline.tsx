import type { ArtifactRef } from "../api/client";
import { agentAvatar } from "./avatars";
import { authorAvatarKey, authorDisplayLabel, visibleAgentLabel } from "./authorDisplay";
import { useI18n } from "../i18n";

// ArtifactByline renders the dual-identity header line for an artifact:
// "<user_avatar> Jaime · via claude-code (opus-4.7)". The user is the
// human who owns the MCP session (users.display_name via
// artifacts.author_user_id); the agent is the string author_id label
// the client reported on propose.
//
// Fallback: artifacts predating migration 0014 (and installs where the
// operator skipped PINDOC_USER_NAME) have author_user=null. In that
// case we show a neutral user slot and keep the avatar anchored to the
// agent identity so fallback markers never show up in list rows.

type Props = {
  artifact: Pick<ArtifactRef, "author_id" | "author_user"> & { author_version?: string };
  // variant controls density; "inline" is the one-line version used on
  // the Reader detail header, "list" is the compact version used in
  // backlink rows where horizontal room is scarce.
  variant?: "inline" | "list";
};

export function ArtifactByline({ artifact, variant = "inline" }: Props) {
  const { t } = useI18n();
  const display = authorDisplayLabel(artifact, t("reader.byline_unknown"));
  const av = agentAvatar(authorAvatarKey(artifact));
  const agent = visibleAgentLabel(artifact);

  if (variant === "list") {
    return (
      <span className="byline byline--list">
        <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>
          {av.initials}
        </span>
        <span>{display}</span>
        {agent && (
          <>
            <span className="byline__sep">·</span>
            <span className="byline__agent">{agent}</span>
          </>
        )}
      </span>
    );
  }

  return (
    <span className="byline">
      <span className={av.className} style={{ width: 18, height: 18, fontSize: 9 }}>
        {av.initials}
      </span>
      <span className="byline__name">{display}</span>
      {agent && (
        <>
          <span className="byline__sep">·</span>
          <span className="byline__agent">
            {t("reader.byline_via", agent)}
          </span>
        </>
      )}
    </span>
  );
}
