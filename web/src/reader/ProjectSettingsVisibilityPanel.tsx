import { Loader2, ShieldAlert } from "lucide-react";
import type { VisibilityTier } from "../api/client";
import { VISIBILITY_TIERS, visibilityLabelKey } from "./visibility";

type Translate = (key: string, ...args: Array<string | number>) => string;

function settingsVisibilityDescriptionKey(tier: VisibilityTier): string {
  return `settings.visibility_${tier}_desc`;
}

function projectVisibilityDescriptionKey(tier: VisibilityTier): string {
  return `settings.project_visibility_${tier}_desc`;
}

function visibilityOptions({
  name,
  value,
  disabled,
  saving,
  t,
  descriptionKey,
  onChange,
}: {
  name: string;
  value: VisibilityTier;
  disabled: boolean;
  saving: boolean;
  t: Translate;
  descriptionKey: (tier: VisibilityTier) => string;
  onChange: (tier: VisibilityTier) => void;
}) {
  return VISIBILITY_TIERS.map((tier) => (
    <label
      className={[
        "project-settings__visibility-option",
        tier === value ? "project-settings__visibility-option--selected" : "",
        disabled ? "project-settings__visibility-option--disabled" : "",
      ].filter(Boolean).join(" ")}
      key={tier}
    >
      <input
        type="radio"
        name={name}
        value={tier}
        checked={tier === value}
        disabled={disabled}
        onChange={() => onChange(tier)}
      />
      <span className="project-settings__visibility-option-copy">
        <span className="project-settings__visibility-option-title">
          {t(visibilityLabelKey(tier))}
          {saving && tier === value && (
            <Loader2 className="lucide project-settings__spinner" aria-hidden />
          )}
        </span>
        <span>{t(descriptionKey(tier))}</span>
      </span>
    </label>
  ));
}

export function ProjectSettingsVisibilityPanel({
  canEdit,
  projectVisibility,
  defaultVisibility,
  saving,
  t,
  onProjectVisibilityChange,
  onDefaultVisibilityChange,
}: {
  canEdit: boolean;
  projectVisibility: VisibilityTier;
  defaultVisibility: VisibilityTier;
  saving: "project" | "default" | null;
  t: Translate;
  onProjectVisibilityChange: (tier: VisibilityTier) => void;
  onDefaultVisibilityChange: (tier: VisibilityTier) => void;
}) {
  const disabled = !canEdit || saving !== null;

  return (
    <section className="project-settings__panel" aria-label={t("settings.project_visibility_section_label")}>
      <div className="project-settings__copy">
        <h2>{t("settings.project_visibility_section_label")}</h2>
        <p>{t("settings.project_visibility_section_desc")}</p>
        {!canEdit && (
          <div className="project-settings__warning" role="alert">
            <ShieldAlert className="lucide" aria-hidden />
            <span>{t("settings.owner_only")}</span>
          </div>
        )}
      </div>

      <div className="project-settings__visibility-stack">
        <fieldset className="project-settings__visibility-group" aria-busy={saving === "project"}>
          <legend>{t("settings.project_visibility_label")}</legend>
          {visibilityOptions({
            name: "project_visibility",
            value: projectVisibility,
            disabled,
            saving: saving === "project",
            t,
            descriptionKey: projectVisibilityDescriptionKey,
            onChange: onProjectVisibilityChange,
          })}
        </fieldset>
        <fieldset className="project-settings__visibility-group" aria-busy={saving === "default"}>
          <legend>{t("settings.visibility_label")}</legend>
          {visibilityOptions({
            name: "default_artifact_visibility",
            value: defaultVisibility,
            disabled,
            saving: saving === "default",
            t,
            descriptionKey: settingsVisibilityDescriptionKey,
            onChange: onDefaultVisibilityChange,
          })}
        </fieldset>
      </div>
    </section>
  );
}
