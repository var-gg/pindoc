import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ArrowLeft, CheckCircle2, Loader2, Settings2, ShieldAlert } from "lucide-react";
import { api, type Project, type ProjectSensitiveOps, type VisibilityTier } from "../api/client";
import { useI18n } from "../i18n";
import { DEFAULT_READER_ORG_SLUG, projectSurfacePath } from "../readerRoutes";
import { EmptyState } from "./SurfacePrimitives";
import { ProjectSettingsVisibilityPanel } from "./ProjectSettingsVisibilityPanel";
import { normalizeVisibilityTier } from "./visibility";
import "../styles/reader.css";

type Notice = {
  kind: "ok" | "err";
  message: string;
};

export function ProjectSettingsPage() {
  const { org, project = "" } = useParams<{ org?: string; project: string }>();
  const { t } = useI18n();
  const [loadedProject, setLoadedProject] = useState<Project | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<"review_queue" | "project_visibility" | "default_visibility" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<Notice | null>(null);
  const orgSlug = org ?? loadedProject?.organization_slug ?? DEFAULT_READER_ORG_SLUG;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setLoadedProject(null);
    api.project(project)
      .then((resp) => {
        if (!cancelled) setLoadedProject(resp);
      })
      .catch((e) => {
        if (!cancelled) {
          setLoadedProject(null);
          setError(String(e));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [project]);

  const sensitiveOps: ProjectSensitiveOps = loadedProject?.sensitive_ops ?? "auto";
  const reviewQueueEnabled = sensitiveOps === "confirm";
  const projectVisibility = normalizeVisibilityTier(loadedProject?.visibility);
  const defaultVisibility = normalizeVisibilityTier(loadedProject?.default_artifact_visibility);
  const canEdit = loadedProject?.current_role === "owner";

  async function setReviewQueueEnabled(nextEnabled: boolean) {
    if (!loadedProject || saving || !canEdit) return;
    const nextMode: ProjectSensitiveOps = nextEnabled ? "confirm" : "auto";
    setSaving("review_queue");
    setNotice(null);
    try {
      const resp = await api.projectSettingsPatch(project, {
        sensitive_ops: nextMode,
      });
      setLoadedProject({ ...loadedProject, sensitive_ops: resp.sensitive_ops ?? nextMode });
      setNotice({
        kind: "ok",
        message: nextEnabled ? t("settings.saved_on") : t("settings.saved_off"),
      });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setNotice({
        kind: "err",
        message: tagged.error_code
          ? `${tagged.error_code}: ${tagged.message}`
          : t("settings.save_error"),
      });
    } finally {
      setSaving(null);
    }
  }

  async function setProjectVisibility(nextVisibility: VisibilityTier) {
    if (!loadedProject || saving || !canEdit || nextVisibility === projectVisibility) return;
    setSaving("project_visibility");
    setNotice(null);
    try {
      const resp = await api.projectSettingsPatch(project, {
        visibility: nextVisibility,
      });
      setLoadedProject({
        ...loadedProject,
        visibility: resp.visibility ?? nextVisibility,
      });
      setNotice({
        kind: "ok",
        message: t("settings.project_visibility_saved"),
      });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setNotice({
        kind: "err",
        message: tagged.error_code
          ? `${tagged.error_code}: ${tagged.message}`
          : t("settings.save_error"),
      });
    } finally {
      setSaving(null);
    }
  }

  async function setDefaultVisibility(nextVisibility: VisibilityTier) {
    if (!loadedProject || saving || !canEdit || nextVisibility === defaultVisibility) return;
    setSaving("default_visibility");
    setNotice(null);
    try {
      const resp = await api.projectSettingsPatch(project, {
        default_artifact_visibility: nextVisibility,
      });
      setLoadedProject({
        ...loadedProject,
        default_artifact_visibility: resp.default_artifact_visibility ?? nextVisibility,
      });
      setNotice({
        kind: "ok",
        message: t("settings.visibility_saved"),
      });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setNotice({
        kind: "err",
        message: tagged.error_code
          ? `${tagged.error_code}: ${tagged.message}`
          : t("settings.save_error"),
      });
    } finally {
      setSaving(null);
    }
  }

  return (
    <main className="project-settings-page">
      <div className="project-settings">
        <Link to={projectSurfacePath(project, "today", undefined, orgSlug)} className="project-settings__back">
          <ArrowLeft className="lucide" aria-hidden />
          {t("settings.back_to_project")}
        </Link>

        <header className="project-settings__header">
          <div className="project-settings__eyebrow">
            <Settings2 className="lucide" aria-hidden />
            {t("surface.name.settings")}
          </div>
          <h1>{t("settings.title")}</h1>
          {loadedProject && (
            <p>
              <span>{loadedProject.name}</span>
              <code>{loadedProject.slug}</code>
            </p>
          )}
        </header>

        {loading && <EmptyState message={t("settings.loading")} />}

        {!loading && error && (
          <EmptyState
            message={t("settings.load_error")}
            action={{ label: error }}
          />
        )}

        {!loading && loadedProject && (
          <section className="project-settings__panel" aria-label={t("settings.review_queue_label")}>
            <div className="project-settings__copy">
              <h2>{t("settings.review_queue_label")}</h2>
              <p>{t("settings.review_queue_desc")}</p>
              {!canEdit && (
                <div className="project-settings__warning" role="alert">
                  <ShieldAlert className="lucide" aria-hidden />
                  <span>{t("settings.owner_only")}</span>
                </div>
              )}
            </div>

            <label className="project-settings__switch-row">
              <input
                className="sr-only"
                type="checkbox"
                checked={reviewQueueEnabled}
                disabled={!canEdit || saving !== null}
                onChange={(e) => void setReviewQueueEnabled(e.target.checked)}
              />
              <span className="project-settings__switch" aria-hidden>
                <span />
              </span>
              <span className="project-settings__switch-state">
                {saving === "review_queue" && <Loader2 className="lucide project-settings__spinner" aria-hidden />}
                {saving !== "review_queue" && reviewQueueEnabled && <CheckCircle2 className="lucide" aria-hidden />}
                {reviewQueueEnabled
                  ? t("settings.review_queue_on")
                  : t("settings.review_queue_off")}
              </span>
            </label>
          </section>
        )}

        {!loading && loadedProject && (
          <ProjectSettingsVisibilityPanel
            canEdit={canEdit}
            projectVisibility={projectVisibility}
            defaultVisibility={defaultVisibility}
            saving={saving === "project_visibility" ? "project" : saving === "default_visibility" ? "default" : null}
            t={t}
            onProjectVisibilityChange={(tier) => void setProjectVisibility(tier)}
            onDefaultVisibilityChange={(tier) => void setDefaultVisibility(tier)}
          />
        )}

        {notice && (
          <div
            className={`project-settings__notice project-settings__notice--${notice.kind}`}
            role={notice.kind === "err" ? "alert" : "status"}
          >
            {notice.message}
          </div>
        )}
      </div>
    </main>
  );
}
