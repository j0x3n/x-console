import { describe, expect, it } from "vitest";
import {
  linesToList,
  problemCount,
  sameAsSaved,
  toDraft,
  toInput,
  type Draft,
} from "./format";
import type { AIConfigState } from "./api";

const tool = { rules: "x", allow: ["a"], ask: [], deny: [], mcp: [] };
const saved: AIConfigState = {
  claude: tool,
  codex: { rules: "", allow: [], ask: [], deny: [], mcp: [] },
  hostIds: ["h2", "h1"],
  hosts: [],
};

describe("aiconfig format B121", () => {
  it("每行一条，去掉空行和重复", () => {
    expect(linesToList(" a \n\nb\na\n  \n")).toEqual(["a", "b"]);
  });

  it("草稿和已保存的一样时不算改动，机器顺序无关", () => {
    expect(sameAsSaved(toDraft(saved), saved)).toBe(true);
    const d: Draft = toDraft(saved);
    expect(sameAsSaved({ ...d, hostIds: ["h1", "h2"] }, saved)).toBe(true);
    expect(
      sameAsSaved({ ...d, claude: { ...d.claude, allow: "a\nb" } }, saved),
    ).toBe(false);
    // 只多了空行不算改动
    expect(
      sameAsSaved({ ...d, claude: { ...d.claude, allow: "a\n\n" } }, saved),
    ).toBe(true);
  });

  it("MCP 服务器按传输方式整理，Codex 不带权限", () => {
    const d = toDraft(saved);
    d.codex.allow = "Bash(ls)";
    d.claude.mcp = [
      {
        name: " fs ",
        transport: "stdio",
        command: " npx ",
        args: "-y\n\npkg",
        url: "ignored",
      },
      {
        name: "docs",
        transport: "http",
        command: "ignored",
        args: "",
        url: " https://x/ ",
      },
    ];
    const input = toInput(d);
    expect(input.codex.allow).toEqual([]);
    expect(input.claude.mcp).toEqual([
      { name: "fs", transport: "stdio", command: "npx", args: ["-y", "pkg"] },
      { name: "docs", transport: "http", url: "https://x/" },
    ]);
  });

  it("数不一致和冲突的机器", () => {
    const h = (state: string) => ({
      hostId: state,
      name: state,
      state,
      items: [],
    });
    expect(
      problemCount([
        h("ok"),
        h("drift"),
        h("drift"),
        h("conflict"),
        h("error"),
      ] as never),
    ).toEqual({ drift: 2, conflict: 1, error: 1 });
    expect(problemCount(undefined)).toEqual({
      drift: 0,
      conflict: 0,
      error: 0,
    });
  });
});
