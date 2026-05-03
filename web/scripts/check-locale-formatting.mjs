import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const srcDir = fileURLToPath(new URL("../src/", import.meta.url));
const bareLocaleCall = /\.toLocale(?:String|DateString|TimeString)\s*\(\s*\)/g;
const sourceExt = /\.(?:ts|tsx)$/;

function* sourceFiles(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      yield* sourceFiles(path);
      continue;
    }
    if (sourceExt.test(entry)) yield path;
  }
}

const failures = [];
for (const file of sourceFiles(srcDir)) {
  const text = readFileSync(file, "utf8");
  for (const match of text.matchAll(bareLocaleCall)) {
    const before = text.slice(0, match.index);
    const line = before.split("\n").length;
    failures.push(`${relative(srcDir, file)}:${line}: ${match[0]}`);
  }
}

if (failures.length > 0) {
  console.error("Bare toLocale* calls are locale-dependent. Use src/utils/formatDateTime.ts helpers instead.");
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
