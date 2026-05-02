import { defaultUrlTransform } from "react-markdown";
import { transformPindocAssetURL } from "./assetReferences";
import { DEFAULT_READER_ORG_SLUG, projectSurfacePath } from "../readerRoutes";

const pindocScheme = "pindoc://";

export function pindocUrlTransform(
  url: string,
  projectSlug?: string,
  orgSlug = DEFAULT_READER_ORG_SLUG,
): string {
  const assetURL = transformPindocAssetURL(url, projectSlug);
  if (assetURL !== null) return assetURL;
  if (!url.startsWith(pindocScheme)) {
    return defaultUrlTransform(url);
  }
  if (!projectSlug) {
    return "";
  }
  const target = url.slice(pindocScheme.length);
  if (!target || target.startsWith("#")) {
    return "";
  }
  const hashIndex = target.indexOf("#");
  const slug = hashIndex >= 0 ? target.slice(0, hashIndex) : target;
  const hash = hashIndex >= 0 ? target.slice(hashIndex) : "";
  if (!slug) {
    return "";
  }
  return `${projectSurfacePath(projectSlug, "wiki", slug, orgSlug)}${hash}`;
}
