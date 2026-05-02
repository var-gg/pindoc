import type { VisibilityTier } from "../api/client";

export const VISIBILITY_TIERS: readonly VisibilityTier[] = ["public", "org", "private"] as const;

export function isVisibilityTier(value: string): value is VisibilityTier {
  return (VISIBILITY_TIERS as readonly string[]).includes(value);
}

export function normalizeVisibilityTier(value: string | null | undefined): VisibilityTier {
  const normalized = (value ?? "").trim().toLowerCase();
  return isVisibilityTier(normalized) ? normalized : "org";
}

export function visibilityLabelKey(tier: VisibilityTier): string {
  return `artifact.visibility.${tier}`;
}

export function visibilityDescriptionKey(tier: VisibilityTier): string {
  return `artifact.visibility.${tier}_desc`;
}

export function visibilityChipClass(tier: VisibilityTier): string {
  return `visibility-chip visibility-chip--${tier}`;
}

export function canEditArtifactVisibility(canEdit: boolean | undefined): boolean {
  return canEdit === true;
}
