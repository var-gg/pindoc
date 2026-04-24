import { ChevronRight } from "lucide-react";
import type { Artifact, ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { ArtifactByline } from "./ArtifactByline";
import { PindocMarkdown } from "./Markdown";
import { TrustCard } from "./TrustCard";
import { localizedAreaName } from "./areaLocale";
import { typeChipClass } from "./typeChip";

type Props = {
  detail: Artifact | null;
  emptyMessage: string;
};

export function ReaderSurface({ detail, emptyMessage }: Props) {
  const { t } = useI18n();

  if (!detail) {
    return (
      <div className="content">
        <div className="surface-stub">
          <p>{emptyMessage}</p>
        </div>
      </div>
    );
  }

  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";
  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);

  return (
    <main className="content">
      <article className="reader-article">
        <div className="crumbs">
          <span>{areaLabel}</span>
          <ChevronRight className="lucide" />
          <span>{detail.type}</span>
          <ChevronRight className="lucide" />
          <span className="current">{detail.slug}</span>
        </div>

        <h1 className="art-title">{detail.title}</h1>

        <TrustCard
          meta={detail.artifact_meta}
          pins={detail.pins}
          taskStatus={detail.type === "Task" ? detail.task_meta?.status : undefined}
          recentWarnings={detail.recent_warnings}
        />

        <div className="art-meta">
          <span className={`chip chip--${detail.status}`}>
            <span className={`p-dot p-dot--${detail.status}`} />
            {detail.status}
          </span>
          <span className={typeChipClass(detail.type)}>{detail.type}</span>
          <span className="chip chip--area">{areaLabel}</span>
          <span className="art-meta__sep">·</span>
          <ArtifactByline artifact={detail} />
          <span className="art-meta__sep">·</span>
          <span className="prov">{t("reader.published", publishedAt)}</span>
        </div>

        <div className="art-body">
          <PindocMarkdown source={detail.body_markdown} />
        </div>

        <RelatedHint detail={detail} />
      </article>
    </main>
  );
}

function RelatedHint({ detail }: { detail: Artifact }) {
  const { t } = useI18n();
  // Real backlinks need artifact_edges table (Phase 3+). For M1 we show
  // a truthful placeholder so the visual treatment stays and the data
  // gap is obvious to the reader.
  return (
    <div className="backlinks">
      <h4>{t("reader.backlinks_empty_head")}</h4>
      <div style={{ fontSize: 13, color: "var(--fg-3)" }}>
        {t("reader.backlinks_empty", detail.slug)}
      </div>
    </div>
  );
}

export function ArtifactListRow({
  artifact,
  isActive,
  to,
}: {
  artifact: ArtifactRef;
  isActive: boolean;
  to: string;
}) {
  // Kept exported because both Reader and Tasks surfaces render lists.
  return (
    <a href={to} className={`wiki__list-row${isActive ? " is-active" : ""}`}>
      <span className="wiki__chip">{artifact.type}</span>
      <span className="wiki__title">{artifact.title}</span>
    </a>
  );
}
