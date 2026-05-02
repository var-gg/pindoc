import { Loader2, ShieldAlert } from "lucide-react";
import type { VisibilityTier } from "../api/client";
import { VISIBILITY_TIERS, visibilityLabelKey } from "./visibility";

type Translate = (key: string, ...args: Array<string | number>) => string;

function settingsVisibilityDescriptionKey(tier: VisibilityTier): string {
  return `settings.visibility_${tier}_desc`;
}

export function ProjectSettingsVisibilityPanel({
  canEdit,
  defaultVisibility,
  saving,
  t,
  onChange,
}: {
  canEdit: boolean;
  defaultVisibility: VisibilityTier;
  saving: boolean;
  t: Translate;
  onChange: (tier: VisibilityTier) => void;
}) {
  const disabled = !canEdit || saving;

  return (
    <section className="project-settings__panel" aria-label={t("settings.visibility_label")}>
      <div className="project-settings__copy">
        <h2>{t("settings.visibility_label")}</h2>
        {!canEdit && (
          <div className="project-settings__warning" role="alert">
            <ShieldAlert className="lucide" aria-hidden />
            <span>{t("settings.owner_only")}</span>
          </div>
        )}
      </div>

      <fieldset className="project-settings__visibility-group" aria-busy={saving}>
        <legend className="sr-only">{t("settings.visibility_label")}</legend>
        {VISIBILITY_TIERS.map((tier) => (
          <label
            className={[
              "project-settings__visibility-option",
              tier === defaultVisibility ? "project-settings__visibility-option--selected" : "",
              disabled ? "project-settings__visibility-option--disabled" : "",
            ].filter(Boolean).join(" ")}
            key={tier}
          >
            <input
              type="radio"
              name="default_artifact_visibility"
              value={tier}
              checked={tier === defaultVisibility}
              disabled={disabled}
              onChange={() => onChange(tier)}
            />
            <span className="project-settings__visibility-option-copy">
              <span className="project-settings__visibility-option-title">
                {t(visibilityLabelKey(tier))}
                {saving && tier === defaultVisibility && (
                  <Loader2 className="lucide project-settings__spinner" aria-hidden />
                )}
              </span>
              <span>{t(settingsVisibilityDescriptionKey(tier))}</span>
            </span>
          </label>
        ))}
      </fieldset>
    </section>
  );
}
