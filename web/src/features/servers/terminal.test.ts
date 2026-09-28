import { describe, expect, it } from "vitest";
import { isRisky, terminalInput } from "./components/TerminalScripts";

describe("terminal scripts (B28)", () => {
  it("wraps bash in a quoted heredoc", () => {
    const body = 'echo "$HOME" `date`\nXC_EOF\n';
    const input = terminalInput({ shell: "bash", body });
    expect(input.startsWith("bash <<'XC_EOF_'\n")).toBe(true);
    expect(input.endsWith("\nXC_EOF_\n")).toBe(true);
    expect(input).toContain('echo "$HOME" `date`');
  });

  it("encodes PowerShell as UTF-16LE base64", () => {
    const input = terminalInput({ shell: "powershell", body: "dir" });
    // "dir" in UTF-16LE is 64 00 69 00 72 00
    expect(input).toBe("powershell -NoProfile -EncodedCommand ZABpAHIA\r");
  });

  it("flags scripts that delete or reboot", () => {
    expect(isRisky("rm -rf /var/cache/x")).toBe(true);
    expect(isRisky("sudo reboot")).toBe(true);
    expect(isRisky("df -h && du -sh /var")).toBe(false);
  });
});
