const pindocAssetScheme = "pindoc-asset://";

export function assetBlobURL(projectSlug: string, assetID: string): string {
  return `/api/p/${encodeURIComponent(projectSlug)}/assets/${encodeURIComponent(assetID)}/blob`;
}

export function transformPindocAssetURL(url: string, projectSlug?: string): string | null {
  if (!url.startsWith(pindocAssetScheme)) return null;
  if (!projectSlug) return "";
  const assetID = url.slice(pindocAssetScheme.length).split(/[?#]/)[0] ?? "";
  if (!assetID) return "";
  return assetBlobURL(projectSlug, assetID);
}
