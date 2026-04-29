import type { ChangeGroup, ChangeGroupArtifactRef, TodayResp } from "../api/client";

type TFn = (key: string, ...args: Array<string | number>) => string;

export type TodayBriefView = {
  sourceLabel: string;
  headline: string;
  bullets: string[];
  fallbackHint: string | null;
};

export type ChangeGroupCardView = {
  kindLabel: string;
  importanceLabel: string;
  verificationLabel: string;
  title: string;
  bullets: string[];
};

export function buildTodayBrief(data: TodayResp, t: TFn): TodayBriefView {
  const groups = data.groups;
  const fallbackHint = fallbackMessage(data, t);
  const headline =
    groups.length === 0
      ? t("today.brief_headline_empty")
      : fallbackHint
        ? t("today.brief_headline_fallback", groups.length)
        : t("today.brief_headline_review", groups.length);

  return {
    sourceLabel: data.summary.source === "llm"
      ? t("today.brief_source_ai")
      : t("today.brief_source_rule"),
    headline,
    bullets: buildBullets(data, t),
    fallbackHint,
  };
}

export function buildChangeGroupCardView(
  group: ChangeGroup,
  t: TFn,
): ChangeGroupCardView {
  return {
    kindLabel: changeKindLabel(group.group_kind, t),
    importanceLabel: importanceLabel(group.importance.level, t),
    verificationLabel: verificationLabel(group.verification_state, t),
    title: representativeChangeGroupTitle(group, t),
    bullets: artifactTitleBullets(group, t),
  };
}

function fallbackMessage(data: TodayResp, t: TFn): string | null {
  switch (data.baseline.fallback_used) {
    case "recent_7d":
      return t("today.fallback_recent_7d");
    case "importance_top":
      return t("today.fallback_importance_top");
    default:
      return null;
  }
}

function fallbackChangeGroupTitle(group: ChangeGroup, t: TFn): string {
  const area = group.areas[0] ?? t("today.change_group_area_fallback");
  return t("today.change_group_title_area", area, group.artifact_count);
}

function representativeArtifacts(group: ChangeGroup): ChangeGroupArtifactRef[] {
  const out: ChangeGroupArtifactRef[] = [];
  const seen = new Set<string>();
  const add = (artifact: ChangeGroupArtifactRef | undefined) => {
    if (!artifact?.title?.trim()) return;
    const key = artifact.id || artifact.slug || artifact.title;
    if (seen.has(key)) return;
    seen.add(key);
    out.push(artifact);
  };
  add(group.first_artifact);
  for (const artifact of group.artifacts ?? []) add(artifact);
  return out;
}

function representativeChangeGroupTitle(group: ChangeGroup, t: TFn): string {
  const artifact = representativeArtifacts(group)[0];
  if (!artifact) return fallbackChangeGroupTitle(group, t);
  const title = trimText(artifact.title, 96);
  const extra = Math.max(0, group.artifact_count - 1);
  const area = group.areas[0] ?? "";
  if (area && extra > 0) {
    return t("today.change_group_title_representative_area_more", area, title, extra);
  }
  if (area) {
    return t("today.change_group_title_representative_area", area, title);
  }
  if (extra > 0) {
    return t("today.change_group_title_representative_more", title, extra);
  }
  return title;
}

function artifactTitleBullets(group: ChangeGroup, t: TFn): string[] {
  const artifacts = representativeArtifacts(group).slice(0, 3);
  if (artifacts.length > 0) {
    return artifacts.map((artifact) =>
      t("today.change_group_bullet_artifact", trimText(artifact.title, 110)),
    );
  }
  if (group.artifact_count > 0) {
    return [t("today.change_group_bullet_artifact_count", group.artifact_count)];
  }
  return [];
}

function trimText(text: string, max: number): string {
  if (text.length <= max) return text;
  return `${text.slice(0, Math.max(0, max - 3)).trimEnd()}...`;
}

function changeKindLabel(kind: ChangeGroup["group_kind"], t: TFn): string {
  switch (kind) {
    case "human_trigger":
      return t("today.kind_human_trigger");
    case "auto_sync":
      return t("today.kind_auto_sync");
    case "maintenance":
      return t("today.kind_maintenance");
    case "system":
      return t("today.kind_system");
  }
}

function importanceLabel(level: ChangeGroup["importance"]["level"], t: TFn): string {
  switch (level) {
    case "high":
      return t("today.importance_high");
    case "medium":
      return t("today.importance_medium");
    case "low":
      return t("today.importance_low");
  }
}

function verificationLabel(state: string, t: TFn): string {
  switch (state) {
    case "verified":
      return t("today.verification_checked");
    case "partially_verified":
      return t("today.verification_partial");
    case "unverified":
      return t("today.verification_needs_review");
    default:
      return t("today.verification_unknown");
  }
}

function buildBullets(data: TodayResp, t: TFn): string[] {
  const groups = data.groups;
  if (groups.length === 0) {
    return [
      t("today.brief_bullet_no_groups"),
      t("today.brief_bullet_no_verification"),
      t("today.brief_bullet_open_history"),
    ];
  }

  const totals = groups.reduce(
    (acc, group) => ({
      revisions: acc.revisions + group.revision_count,
      artifacts: acc.artifacts + group.artifact_count,
      verificationRisk:
        acc.verificationRisk ||
        group.verification_state === "unverified" ||
        group.verification_state === "partially_verified",
    }),
    { revisions: 0, artifacts: 0, verificationRisk: false },
  );

  return [
    t("today.brief_bullet_top", groups[0] ? representativeChangeGroupTitle(groups[0], t) : ""),
    t("today.brief_bullet_counts", groups.length, totals.revisions, totals.artifacts),
    totals.verificationRisk
      ? t("today.brief_bullet_verification_risk")
      : t("today.brief_bullet_no_verification"),
  ];
}
