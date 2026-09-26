// Generates TypeScript types for every module spec in ../api/modules.
// openapi-typescript needs TypeScript 5, so it runs through npx in its own
// environment instead of this project's TypeScript 7.
import { execFileSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { basename, join } from "node:path";

const specDir = join(import.meta.dirname, "..", "..", "api", "modules");
const outDir = join(import.meta.dirname, "..", "src", "api", "gen");
const only = process.argv[2];
for (const file of readdirSync(specDir).filter((f) => f.endsWith(".yaml"))) {
  const name = basename(file, ".yaml");
  if (only && only !== name) continue;
  execFileSync(
    "npx",
    [
      "--yes",
      "-p",
      "openapi-typescript@7.13.0",
      "-p",
      "typescript@5.9",
      "openapi-typescript",
      join(specDir, file),
      "-o",
      join(outDir, `${name}.ts`),
    ],
    { stdio: "inherit", shell: process.platform === "win32" },
  );
}
