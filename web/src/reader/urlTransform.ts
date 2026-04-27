import { defaultUrlTransform } from "react-markdown";

const pindocScheme = "pindoc://";

export function pindocUrlTransform(url: string, projectSlug?: string): string {
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
  return `/p/${projectSlug}/wiki/${slug}${hash}`;
}
