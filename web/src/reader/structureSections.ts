import { headingsFromBody, slugifyHeading } from "./slug";

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

export type StructureOverlapSection = {
  title: string;
  slug: string;
  body: string;
};

export function structureOverlapSectionsFromBody(source: string): StructureOverlapSection[] {
  const headings = headingsFromBody(source);
  if (headings.length === 0) return [];

  const lines = source.split(/\r?\n/);
  const sections: StructureOverlapSection[] = [];
  let headingIndex = 0;
  let inFence = false;

  for (let i = 0; i < lines.length; ) {
    const line = lines[i].trimEnd();
    if (/^```/.test(line.trim())) {
      inFence = !inFence;
      i += 1;
      continue;
    }
    const m = !inFence ? /^##\s+(.+?)\s*#*\s*$/.exec(line) : null;
    if (!m) {
      i += 1;
      continue;
    }

    const title = m[1].trim();
    const heading = headings[headingIndex++];
    const slug = heading?.slug ?? slugifyHeading(title);
    let end = i + 1;
    let sectionFence = false;
    while (end < lines.length) {
      const next = lines[end].trimEnd();
      if (/^```/.test(next.trim())) {
        sectionFence = !sectionFence;
      } else if (!sectionFence && /^##\s+(.+?)\s*#*\s*$/.test(next)) {
        break;
      }
      end += 1;
    }

    if (isStructureOverlapHeading(title)) {
      sections.push({
        title,
        slug,
        body: lines.slice(i + 1, end).join("\n").trim(),
      });
    }
    i = end;
  }

  return sections;
}

function normalizeStructureHeading(heading: string): string {
  return heading
    .toLowerCase()
    .replace(/[`*_~]/g, "")
    .replace(/\s*\/\s*/g, "/")
    .replace(/\s+/g, " ")
    .trim();
}
