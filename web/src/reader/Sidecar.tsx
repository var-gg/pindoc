import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { ArrowUpRight, History as HistoryIcon } from "lucide-react";
import {
  api,
  type Artifact,
  type ArtifactMeta,
  type EdgeRef,
  type PinRef,
  type RevisionRow,
  type ServerConfig,
  type SourceSessionRef,
  type UserRef,
} from "../api/client";
import type { Aggregate } from "./useReaderData";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";
import { TaskControls } from "./TaskControls";
import { Toc } from "./Toc";
import { headingsFromBody } from "./slug";

type Props = {
  projectSlug: string;
  detail: Artifact | null;
  // auth_mode lets TaskControls flip between inline-editable and
  // read-only without a second round-trip (Decision agent-only-write-
  // 분할). Undefined = config not yet loaded — treat as non-trusted and
  // stay read-only until we know.
  authMode?: ServerConfig["auth_mode"];
  // agents is the author_id aggregate across the current project's
  // artifact list. TaskControls surfaces it as the "assigned to an
  // agent" half of the assignee dropdown.
  agents?: Aggregate[];
  // users is the instance-wide users table projection. Combined with
  // agents to build the assignee dropdown — null when the /api/users
  // fetch failed (Reader still renders, TaskControls hides the users
  // section).
  users?: UserRef[] | null;
  // onArtifactUpdated is called after a successful task-meta write so
  // the Reader refetches the detail and the revision rail / TaskControls
  // reflect the new head.
  onArtifactUpdated?: () => void;
};

export function Sidecar({ projectSlug, detail, authMode, agents, users, onArtifactUpdated }: Props) {
  const { t } = useI18n();
  // TOC feeds off body_markdown; Markdown.tsx independently derives the
  // same slugs via the same uniqueSlug ledger so `<h2 id>` matches
  // `href="#..."` without a DOM round-trip. Computed here (not ReaderSurface)
  // so Sidecar owns the TOC lifecycle alongside its other metadata rails.
  const headings = useMemo(
    () => (detail ? headingsFromBody(detail.body_markdown) : []),
    [detail],
  );

  if (!detail) {
    return (
      <aside className="sidecar sidecar--empty">
        <div className="sidecar__head">
          <h3>{t("sidecar.this_artifact")}</h3>
        </div>
        <div className="graph-wrap">
          <div className="mini-graph">
            <span className="mini-graph__empty">{t("sidecar.no_selection")}</span>
          </div>
        </div>
      </aside>
    );
  }

  const av = agentAvatar(detail.author_id);
  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";

  // Graph edges aren't derived yet (Phase 3+ pipeline populates these via
  // artifact.superseded_by + future artifact_edges). Show placeholder
  // states so the visual treatment is faithful and the data gap is
  // honest.
  const hasSupersedes = Boolean(detail.superseded_by && detail.superseded_by !== "");

  return (
    <aside className="sidecar sidecar--detail">
      <div className="sidecar__head">
        <h3>{t("sidecar.this_artifact")}</h3>
      </div>

      {headings.length >= 2 && <Toc headings={headings} />}

      {detail.type === "Task" && (
        <TaskControls
          projectSlug={projectSlug}
          detail={detail}
          authMode={authMode}
          agents={agents ?? []}
          users={users ?? []}
          onUpdated={() => onArtifactUpdated?.()}
        />
      )}

      <div className="graph-wrap">
        <div className="mini-graph">
          <svg viewBox="0 0 280 200" style={{ width: "100%", height: "100%" }}>
            <g stroke="currentColor" strokeWidth="1" fill="none" opacity="0.35" style={{ color: "var(--fg-3)" }}>
              <line x1="140" y1="100" x2="60" y2="60" />
              <line x1="140" y1="100" x2="220" y2="50" />
              <line x1="140" y1="100" x2="220" y2="150" />
              <line x1="140" y1="100" x2="60" y2="150" />
            </g>
            <g fontFamily="JetBrains Mono, monospace" fontSize="9">
              <g transform="translate(140,100)">
                <circle r="14" fill="var(--live-bg)" stroke="var(--live)" strokeWidth="1.5" />
                <text textAnchor="middle" dy="3" fill="var(--live)">
                  {(detail.type.slice(0, 3) || "A").toUpperCase()}
                </text>
              </g>
              <g transform="translate(60,60)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(220,50)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(220,150)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(60,150)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
            </g>
          </svg>
        </div>
      </div>

      <ConnectedArtifacts
        projectSlug={projectSlug}
        relates={detail.relates_to ?? []}
        relatedBy={detail.related_by ?? []}
        hasSupersedes={hasSupersedes}
        supersededBy={detail.superseded_by ?? ""}
      />

      <ProvenanceBlock
        meta={detail.artifact_meta}
        pins={detail.pins}
        sourceSession={detail.source_session_ref}
        updatedAt={detail.updated_at}
      />

      <RecentChanges projectSlug={projectSlug} slug={detail.slug} />

      <div className="provenance">
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_author")}</span>
          <span className="v" style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>
              {av.initials}
            </span>
            {detail.author_id}
            {detail.author_version ? `@${detail.author_version}` : ""}
          </span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_area")}</span>
          <span className="v">{detail.area_slug}</span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_status")}</span>
          <span className="v">
            {detail.status} · {detail.completeness}
          </span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_published")}</span>
          <span className="v">{publishedAt}</span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_id")}</span>
          <span className="v">{detail.slug}</span>
        </div>
      </div>
    </aside>
  );
}

function RecentChanges({ projectSlug, slug }: { projectSlug: string; slug: string }) {
  const { t } = useI18n();
  const { locale = "" } = useParams<{ locale?: string }>();
  const [revs, setRevs] = useState<RevisionRow[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await api.revisions(projectSlug, slug);
        if (!cancelled) setRevs(resp.revisions);
      } catch {
        if (!cancelled) setRevs([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectSlug, slug]);

  if (!revs || revs.length === 0) return null;
  const shown = revs.slice(0, 3);
  const remainder = revs.length - shown.length;

  return (
    <div className="provenance" style={{ paddingBottom: 6 }}>
      <div style={{
        display: "flex", alignItems: "center", justifyContent: "space-between",
        marginBottom: 8,
      }}>
        <div className="provenance__row" style={{ gridTemplateColumns: "1fr", margin: 0, padding: 0 }}>
          <span className="k" style={{ textTransform: "uppercase", letterSpacing: "0.04em" }}>
            {t("history.recent_changes")}
          </span>
        </div>
        <Link
          to={`/p/${projectSlug}/${locale}/wiki/${slug}/history`}
          style={{ color: "var(--fg-2)", textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 4, fontSize: 11, fontFamily: "var(--font-mono)" }}
        >
          <HistoryIcon className="lucide" style={{ width: 11, height: 11 }} />
          {shown.length < revs.length ? t("history.count", revs.length) : ""}
        </Link>
      </div>
      {shown.map((r) => {
        const av = agentAvatar(r.author_id);
        return (
          <div key={r.revision_number} style={{ padding: "4px 0", fontSize: 12 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--fg-1)" }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)" }}>
                rev {r.revision_number}
              </span>
              <span className={av.className} style={{ width: 12, height: 12, fontSize: 7 }}>
                {av.initials}
              </span>
              <span style={{ color: "var(--fg-2)", fontSize: 11.5, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {r.commit_msg || t("history.no_commit_msg")}
              </span>
            </div>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-4)", marginLeft: 36 }}>
              {new Date(r.created_at).toLocaleString()}
            </div>
          </div>
        );
      })}
      {remainder > 0 && (
        <Link
          to={`/p/${projectSlug}/${locale}/wiki/${slug}/history`}
          style={{ fontSize: 11, color: "var(--fg-3)", fontFamily: "var(--font-mono)", textDecoration: "none" }}
        >
          {t("history.more_revisions", remainder)}
        </Link>
      )}
    </div>
  );
}

// ConnectedArtifacts renders typed edges (relates_to / related_by) as
// clickable cards. This is the "Task hub → Wiki spoke" navigation layer
// — a user viewing a Task sees every Decision / Analysis / Debug it
// points to (or is pointed to by) one click away. Supersede relationship
// still uses the dedicated field since it has product meaning beyond a
// plain edge.
function ConnectedArtifacts({
  projectSlug,
  relates,
  relatedBy,
  hasSupersedes,
  supersededBy,
}: {
  projectSlug: string;
  relates: EdgeRef[];
  relatedBy: EdgeRef[];
  hasSupersedes: boolean;
  supersededBy: string;
}) {
  const { t } = useI18n();
  const nothing = !hasSupersedes && relates.length === 0 && relatedBy.length === 0;

  if (nothing) {
    return (
      <div className="relations">
        <div className="relations__empty">{t("sidecar.no_relations")}</div>
      </div>
    );
  }

  return (
    <div className="relations" style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {hasSupersedes && (
        <div className="relation">
          <span className="relation__label">{t("sidecar.rel_supersedes")}</span>
          <span className="relation__target">
            <ArrowUpRight className="lucide" />
            {supersededBy}
          </span>
        </div>
      )}
      {relates.length > 0 && (
        <EdgeGroup
          title={t("sidecar.rel_outgoing")}
          edges={relates}
          projectSlug={projectSlug}
        />
      )}
      {relatedBy.length > 0 && (
        <EdgeGroup
          title={t("sidecar.rel_incoming")}
          edges={relatedBy}
          projectSlug={projectSlug}
        />
      )}
    </div>
  );
}

// ProvenanceBlock renders the epistemic + evidence data the Trust Card
// alludes to: pins list grouped by kind, source_session_ref summary, stale
// signal, and next_context_policy rationale. Section is skipped entirely
// when none of the signals carry information — legacy artifacts without
// artifact_meta keep the old sidecar layout untouched.
function ProvenanceBlock({
  meta,
  pins,
  sourceSession,
  updatedAt,
}: {
  meta?: ArtifactMeta;
  pins?: PinRef[];
  sourceSession?: SourceSessionRef;
  updatedAt: string;
}) {
  const { t } = useI18n();
  const hasPins = (pins?.length ?? 0) > 0;
  const hasSource = Boolean(sourceSession && (sourceSession.agent_id || sourceSession.source_session));
  const hasMetaContent = meta && Object.keys(meta).length > 0;
  const stale = staleFromAge(updatedAt);
  if (!hasPins && !hasSource && !hasMetaContent && !stale) {
    return null;
  }

  return (
    <div className="provenance" style={{ display: "flex", flexDirection: "column", gap: 10, paddingBottom: 6 }}>
      <div
        style={{
          fontFamily: "var(--font-mono)",
          fontSize: 10,
          color: "var(--fg-3)",
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        {t("sidecar.provenance") || "Provenance"}
      </div>

      {hasPins && <PinsList pins={pins!} />}
      {hasSource && <SourceSessionLine session={sourceSession!} />}
      {hasMetaContent && <NextContextLine meta={meta!} />}
      {stale && <StaleLine reason={stale.reason} days={stale.days} />}
    </div>
  );
}

function PinsList({ pins }: { pins: PinRef[] }) {
  const groups = new Map<string, PinRef[]>();
  for (const p of pins) {
    const key = p.kind || "code";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(p);
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <div style={{ fontSize: 11, color: "var(--fg-2)" }}>Pins · {pins.length}</div>
      <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 3 }}>
        {Array.from(groups.entries()).flatMap(([kind, items]) =>
          items.map((p, idx) => (
            <li
              key={`${kind}-${idx}-${p.path}`}
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                color: "var(--fg-1)",
                display: "flex",
                gap: 6,
                alignItems: "baseline",
              }}
              title={`${kind} pin`}
            >
              <span className="chip" style={{ fontSize: 9, textTransform: "uppercase", padding: "1px 5px" }}>
                {kind}
              </span>
              <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {p.path}
                {p.lines_start
                  ? `:${p.lines_start}${p.lines_end && p.lines_end !== p.lines_start ? `-${p.lines_end}` : ""}`
                  : ""}
              </span>
            </li>
          )),
        )}
      </ul>
    </div>
  );
}

function SourceSessionLine({ session }: { session: SourceSessionRef }) {
  const agent = session.agent_id || session.reported_author_id;
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Session: </span>
      {agent ? (
        <span style={{ fontFamily: "var(--font-mono)" }}>{agent}</span>
      ) : (
        <span style={{ color: "var(--fg-3)" }}>ephemeral — not recorded</span>
      )}
      {session.source_session ? (
        <span
          style={{ color: "var(--fg-3)", fontFamily: "var(--font-mono)", marginLeft: 6 }}
          title={session.source_session}
        >
          · {session.source_session.slice(0, 10)}…
        </span>
      ) : null}
    </div>
  );
}

function NextContextLine({ meta }: { meta: ArtifactMeta }) {
  const policy = meta.next_context_policy ?? "default";
  const label =
    policy === "excluded"
      ? "Excluded from next session"
      : policy === "opt_in"
      ? "Next session: opt-in only"
      : "Next session: default";
  const why =
    policy === "excluded"
      ? "Agents won't see this in Fast Landing."
      : policy === "opt_in"
      ? "Surfaces only on direct retrieval."
      : "Eligible for default Fast Landing bundle.";
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Context: </span>
      {label}
      <span style={{ color: "var(--fg-3)", marginLeft: 6 }}>· {why}</span>
    </div>
  );
}

function StaleLine({ reason, days }: { reason: string; days: number }) {
  return (
    <div style={{ fontSize: 11, color: "var(--fg-2)" }}>
      <span style={{ color: "var(--fg-3)" }}>Stale signal: </span>
      <span>{reason}</span>
      <span style={{ color: "var(--fg-3)", marginLeft: 6 }}>({days}d)</span>
    </div>
  );
}

// staleFromAge mirrors the server's Phase 11c age heuristic so the
// Sidecar can surface the signal without a second HTTP round-trip.
// Threshold is 60 days — matches staleAgeThreshold in
// internal/pindoc/mcp/tools/context_for_task.go. When the server eventually
// emits a richer stale_reason enum (pin_changed, ia_migration, etc.) this
// helper becomes a renderer and the enum wins.
function staleFromAge(updatedAt: string): { reason: string; days: number } | null {
  const t = new Date(updatedAt).getTime();
  if (!Number.isFinite(t)) return null;
  const days = Math.floor((Date.now() - t) / (1000 * 60 * 60 * 24));
  if (days <= 60) return null;
  return { reason: `not updated in ${days} days`, days };
}

function EdgeGroup({
  title,
  edges,
  projectSlug,
}: {
  title: string;
  edges: EdgeRef[];
  projectSlug: string;
}) {
  const { locale = "" } = useParams<{ locale?: string }>();
  return (
    <div>
      <div
        style={{
          fontFamily: "var(--font-mono)",
          fontSize: 10,
          color: "var(--fg-3)",
          textTransform: "uppercase",
          letterSpacing: "0.04em",
          marginBottom: 4,
        }}
      >
        {title}
      </div>
      <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 4 }}>
        {edges.map((e) => (
          <li key={`${e.relation}-${e.artifact_id}`}>
            <Link
              to={`/p/${projectSlug}/${locale}/wiki/${e.slug}`}
              style={{
                display: "flex",
                gap: 6,
                alignItems: "baseline",
                padding: "4px 6px",
                borderRadius: "var(--r-1)",
                textDecoration: "none",
                color: "var(--fg-1)",
                fontSize: 12,
              }}
            >
              <span
                className="chip"
                style={{
                  fontSize: 9,
                  textTransform: "uppercase",
                  letterSpacing: "0.03em",
                }}
                title={e.relation}
              >
                {e.relation}
              </span>
              <span style={{ fontSize: 10, color: "var(--fg-3)", fontFamily: "var(--font-mono)" }}>{e.type}</span>
              <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1 }}>{e.title}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
