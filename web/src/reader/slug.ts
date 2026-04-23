// slug — deterministic heading-id generator used by both the markdown
// renderer (Markdown.tsx) and the sticky TOC (Toc.tsx). Same function on
// both sides keeps `<h2 id="...">` and `href="#..."` in lock-step; the
// TOC never has to ask the DOM what id the heading rendered with.
//
// Behaviour:
//   - lowercases ASCII letters
//   - keeps Hangul jamo and hanja so "## 목적 / Purpose" → "목적-purpose"
//   - strips everything else that isn't a word char, space, dash, or CJK
//   - collapses whitespace runs into a single dash
//   - trims leading/trailing dashes
//   - de-duplicates identical slugs within a document via a counter
//     (caller passes a Set and we suffix "-2", "-3" as needed)

export function slugifyHeading(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s가-힣\u4e00-\u9fff-]/g, "")
    .trim()
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function uniqueSlug(text: string, used: Set<string>): string {
  const base = slugifyHeading(text);
  if (!used.has(base)) {
    used.add(base);
    return base;
  }
  for (let i = 2; ; i += 1) {
    const candidate = `${base}-${i}`;
    if (!used.has(candidate)) {
      used.add(candidate);
      return candidate;
    }
  }
}

// headingsFromBody pulls every `## heading` line out of a markdown body
// in order. Skips H2s inside fenced code blocks so a `## not a heading`
// comment in a code example doesn't sneak into the TOC.
export function headingsFromBody(body: string): Array<{ text: string; slug: string }> {
  const lines = body.split(/\r?\n/);
  const out: Array<{ text: string; slug: string }> = [];
  const used = new Set<string>();
  let inFence = false;
  for (const raw of lines) {
    const line = raw.trimEnd();
    if (/^```/.test(line.trim())) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const m = /^##\s+(.+?)\s*#*\s*$/.exec(line);
    if (!m) continue;
    const text = m[1].trim();
    out.push({ text, slug: uniqueSlug(text, used) });
  }
  return out;
}
