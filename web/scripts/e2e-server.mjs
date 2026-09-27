import { randomBytes } from "node:crypto";
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const dataDir = mkdtempSync(path.join(tmpdir(), "x-console-e2e-server-"));
const binary = path.resolve("../backend/bin/e2e-server" + (process.platform === "win32" ? ".exe" : ""));
const child = spawn(binary, [], {
  stdio: "inherit",
  env: {
    ...process.env,
    XC_ADDR: `127.0.0.1:${process.env.XC_E2E_API_PORT ?? "18080"}`,
    XC_DATA_DIR: dataDir,
    XC_MASTER_KEY: randomBytes(32).toString("base64"),
    XC_DEV: "1",
  },
});

let stopping = false;
function stop() {
  if (stopping) return;
  stopping = true;
  child.kill();
}
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
child.on("error", (error) => {
  console.error(error);
  process.exitCode = 1;
});
child.on("exit", (code) => {
  rmSync(dataDir, { recursive: true, force: true });
  process.exit(code ?? 0);
});
