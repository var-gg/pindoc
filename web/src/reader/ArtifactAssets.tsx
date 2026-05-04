import { AlertTriangle, Download, FileText, ImageIcon, Paperclip } from "lucide-react";
import type { AssetRef, VisibilityTier } from "../api/client";
import { useI18n } from "../i18n";

type VisibleAssetRole = "evidence" | "generated_output" | "attachment";

type AssetGroup = {
  role: VisibleAssetRole;
  assets: AssetRef[];
};

const assetRoleOrder: VisibleAssetRole[] = ["evidence", "generated_output", "attachment"];
const visibilityOrder: VisibilityTier[] = ["public", "org", "private"];

export function ArtifactAssets({ assets }: { assets: AssetRef[] }) {
  const { t } = useI18n();
  const groups = groupVisibleAssetsByRole(assets);
  const inlineWarningLabel = inlineImageCrossVisibilityLabel(assets, t);
  if (groups.length === 0 && !inlineWarningLabel) return null;
  return (
    <section className="artifact-assets" aria-label={t("reader.assets_title")}>
      <div className="artifact-assets__head">
        <Paperclip className="lucide artifact-assets__paperclip" aria-hidden="true" />
        <h2>{t("reader.assets_title")}</h2>
      </div>
      {inlineWarningLabel ? (
        <div className="artifact-assets__inline-warning" role="note">
          <AlertTriangle className="lucide" aria-hidden="true" />
          <span>{inlineWarningLabel}</span>
        </div>
      ) : null}
      <div className="artifact-assets__groups">
        {groups.map((group) => (
          <div
            key={group.role}
            className={`artifact-assets__group artifact-assets__group--${group.role}`}
          >
            <h3 className="artifact-assets__group-title">
              {t(assetGroupLabelKey(group.role))} <span>· {group.assets.length}</span>
            </h3>
            <div className="artifact-assets__list">
              {group.assets.map((asset) => {
                const Icon = asset.is_image ? ImageIcon : FileText;
                const label = asset.original_filename || asset.id;
                const warningLabel = crossVisibilityLabel(asset, t);
                return (
                  <a
                    key={`${asset.role}-${asset.id}-${asset.display_order}`}
                    className="artifact-asset"
                    href={asset.blob_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <span className="artifact-asset__icon">
                      <Icon className="lucide" aria-hidden="true" />
                    </span>
                    <span className="artifact-asset__body">
                      <span className="artifact-asset__name">{label}</span>
                      <span className="artifact-asset__meta">{formatAssetMeta(asset)}</span>
                      {warningLabel ? (
                        <span className="artifact-asset__warning" title={warningLabel}>
                          <AlertTriangle className="lucide" aria-hidden="true" />
                          <span>{warningLabel}</span>
                        </span>
                      ) : null}
                    </span>
                    <Download className="lucide artifact-asset__download" aria-hidden="true" />
                  </a>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

export function groupVisibleAssetsByRole(assets: AssetRef[]): AssetGroup[] {
  const buckets: Record<VisibleAssetRole, AssetRef[]> = {
    evidence: [],
    generated_output: [],
    attachment: [],
  };

  for (const asset of assets) {
    if (asset.role === "inline_image") continue;
    buckets[asset.role].push(asset);
  }

  return assetRoleOrder
    .map((role) => ({
      role,
      assets: [...buckets[role]].sort((a, b) => a.display_order - b.display_order),
    }))
    .filter((group) => group.assets.length > 0);
}

function assetGroupLabelKey(role: VisibleAssetRole): string {
  switch (role) {
    case "evidence":
      return "reader.asset_group_evidence";
    case "generated_output":
      return "reader.asset_group_generated_output";
    case "attachment":
      return "reader.asset_group_attachment";
  }
}

function formatAssetMeta(asset: AssetRef): string {
  return [asset.mime_type, formatAssetBytes(asset.size_bytes)]
    .filter((part) => part.length > 0)
    .join(" · ");
}

function inlineImageCrossVisibilityLabel(
  assets: AssetRef[],
  t: (key: string, ...args: Array<string | number>) => string,
): string {
  const inlineVisibility = new Set<VisibilityTier>();
  for (const asset of assets) {
    if (asset.role !== "inline_image") continue;
    for (const tier of sortedCrossVisibility(asset.cross_visibility)) {
      inlineVisibility.add(tier);
    }
  }
  const labels = visibilityOrder
    .filter((tier) => inlineVisibility.has(tier))
    .map((tier) => t(crossVisibilityTierLabelKey(tier)));
  if (labels.length === 0) return "";
  return t("reader.asset_cross_visibility_inline_banner", labels.join(", "));
}

function crossVisibilityLabel(
  asset: AssetRef,
  t: (key: string, ...args: Array<string | number>) => string,
): string {
  const labels = sortedCrossVisibility(asset.cross_visibility).map((tier) =>
    t(crossVisibilityTierLabelKey(tier)),
  );
  if (labels.length === 0) return "";
  return t("reader.asset_cross_visibility_badge", labels.join(", "));
}

function sortedCrossVisibility(visibility: AssetRef["cross_visibility"]): VisibilityTier[] {
  if (!visibility || visibility.length === 0) return [];
  const tiers = new Set<VisibilityTier>();
  for (const tier of visibility) {
    if (visibilityOrder.includes(tier)) {
      tiers.add(tier);
    }
  }
  return visibilityOrder.filter((tier) => tiers.has(tier));
}

function crossVisibilityTierLabelKey(tier: VisibilityTier): string {
  return `reader.asset_cross_visibility.${tier}`;
}

function formatAssetBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}
