export const STRUCTURE_OVERLAP_HEADINGS = [
  "연관",
  "관련",
  "관련 artifact",
  "관련 artifacts",
  "역참조",
  "backlinks",
  "dependencies",
  "dependencies / 선후",
  "dependencies/선후",
  "선후",
  "리소스 경로",
  "resource path",
  "resource paths",
  "related",
  "related artifacts",
  "references",
] as const;

const STRUCTURE_OVERLAP_SET = new Set(
  STRUCTURE_OVERLAP_HEADINGS.map(normalizeStructureHeading),
);

export function isStructureOverlapHeading(heading: string): boolean {
  return STRUCTURE_OVERLAP_SET.has(normalizeStructureHeading(heading));
}

function normalizeStructureHeading(heading: string): string {
  return heading
    .toLowerCase()
    .replace(/[`*_~]/g, "")
    .replace(/\s*\/\s*/g, "/")
    .replace(/\s+/g, " ")
    .trim();
}
