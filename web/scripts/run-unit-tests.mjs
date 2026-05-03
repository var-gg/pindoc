import { readdir } from "node:fs/promises";
import path from "node:path";
import { createServer } from "vite";

const testDir = path.resolve("tests");
const testFiles = (await readdir(testDir))
  .filter((name) => /\.test\.tsx?$/.test(name))
  .sort((a, b) => a.localeCompare(b, "en"));

if (testFiles.length === 0) {
  console.error("No unit tests found in web/tests.");
  process.exitCode = 1;
} else {
  const server = await createServer({
    root: process.cwd(),
    logLevel: "error",
    server: { middlewareMode: true },
    ssr: {
      noExternal: ["lucide-react", "mermaid", "react-markdown", "remark-gfm"],
    },
  });

  try {
    for (const file of testFiles) {
      await server.ssrLoadModule(`/tests/${file}`);
      console.log(`PASS ${file}`);
    }
  } catch (err) {
    console.error(err);
    process.exitCode = 1;
  } finally {
    await server.close();
  }
}
