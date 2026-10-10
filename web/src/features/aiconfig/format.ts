import type {
  AIConfigHostStatus,
  AIConfigInput,
  AIConfigItem,
  AIConfigMcp,
  AIConfigState,
  AIConfigTool,
} from "./api";

/** 英文键，中文在 i18n.ts。 */
export const TOOL_LABELS: Record<string, string> = {
  claude: "Claude Code",
  codex: "Codex",
};

export const ITEM_LABELS: Record<string, string> = {
  rules: "Rules",
  permissions: "Tool permissions",
  mcp: "MCP servers",
};

export const HOST_STATE_LABELS: Record<AIConfigHostStatus["state"], string> = {
  ok: "In sync",
  drift: "Out of sync",
  conflict: "Conflict",
  absent: "Not installed",
  offline: "Offline",
  unsupported: "Not supported",
  error: "Check failed",
};

export function hostTone(state: AIConfigHostStatus["state"]): string {
  if (state === "ok") return "ok";
  if (state === "drift") return "warn";
  if (state === "conflict" || state === "error") return "danger";
  return "";
}

/** 一项不一致或冲突的原因，英文键。没有原因返回空。 */
export function reasonLabel(item: AIConfigItem): string {
  switch (item.reason) {
    case "missing":
      return "Something is missing";
    case "extra":
      return "Something was removed in the panel but is still there";
    case "different":
      return "The content differs";
    case "exists":
      return "A server of this name is already there and was not written by the panel";
    case "invalid":
      return "The file can not be read";
    case "markers":
      return "The begin and end markers do not match";
  }
  return "";
}

/** 每行一条：去掉空行和两端空白，重复的只留一条。 */
export function linesToList(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const line of text.split("\n")) {
    const v = line.trim();
    if (v && !seen.has(v)) {
      seen.add(v);
      out.push(v);
    }
  }
  return out;
}

export function listToLines(list: string[] | undefined): string {
  return (list ?? []).join("\n");
}

/** 编辑中的一个 MCP 服务器。参数用换行分隔，方便写带空格的参数。 */
export interface McpDraft {
  name: string;
  transport: "stdio" | "http";
  command: string;
  args: string;
  url: string;
}

export interface ToolDraft {
  rules: string;
  allow: string;
  ask: string;
  deny: string;
  mcp: McpDraft[];
}

export interface Draft {
  claude: ToolDraft;
  codex: ToolDraft;
  hostIds: string[];
}

export function toolDraft(t: AIConfigTool): ToolDraft {
  return {
    rules: t.rules ?? "",
    allow: listToLines(t.allow),
    ask: listToLines(t.ask),
    deny: listToLines(t.deny),
    mcp: (t.mcp ?? []).map((s) => ({
      name: s.name,
      transport: s.transport,
      command: s.command ?? "",
      args: listToLines(s.args),
      url: s.url ?? "",
    })),
  };
}

export function toDraft(s: AIConfigState): Draft {
  return {
    claude: toolDraft(s.claude),
    codex: toolDraft(s.codex),
    hostIds: [...s.hostIds],
  };
}

function toTool(d: ToolDraft): AIConfigTool {
  const mcp: AIConfigMcp[] = d.mcp.map((s) =>
    s.transport === "http"
      ? { name: s.name.trim(), transport: "http", url: s.url.trim() }
      : {
          name: s.name.trim(),
          transport: "stdio",
          command: s.command.trim(),
          args: linesToList(s.args),
        },
  );
  return {
    rules: d.rules.trim(),
    allow: linesToList(d.allow),
    ask: linesToList(d.ask),
    deny: linesToList(d.deny),
    mcp,
  };
}

export function toInput(d: Draft): AIConfigInput {
  return {
    claude: toTool(d.claude),
    codex: { ...toTool(d.codex), allow: [], ask: [], deny: [] },
    hostIds: [...d.hostIds].sort(),
  };
}

/** 编辑中的内容和已保存的是否一样，只看会存下去的部分。 */
export function sameAsSaved(d: Draft, saved: AIConfigState): boolean {
  const norm = (i: AIConfigInput) => JSON.stringify(i);
  return norm(toInput(d)) === norm(toInput(toDraft(saved)));
}

export function emptyMcp(): McpDraft {
  return { name: "", transport: "stdio", command: "", args: "", url: "" };
}

/** 页头副标题要的概况：不一致和冲突的机器数。 */
export function problemCount(hosts: AIConfigHostStatus[] | undefined) {
  let drift = 0;
  let conflict = 0;
  let error = 0;
  for (const h of hosts ?? []) {
    if (h.state === "drift") drift++;
    else if (h.state === "conflict") conflict++;
    else if (h.state === "error") error++;
  }
  return { drift, conflict, error };
}
