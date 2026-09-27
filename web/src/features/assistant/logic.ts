import type { ContentBlock, Message, PendingAction, Tool } from "./api";

export type ActionStatus = "running" | "waiting" | "ok" | "error" | "rejected";

export type TimelineItem =
  | { kind: "user"; key: string; text: string }
  | { kind: "assistant"; key: string; text: string; streaming?: boolean }
  | {
      kind: "action";
      key: string;
      action: string;
      title: string;
      effect: Tool["effect"] | "unknown";
      input: Record<string, unknown>;
      status: ActionStatus;
      result?: unknown;
      pending?: PendingAction;
    };

/** 工具名把 . 换成了 __，换回动作名。 */
export function actionName(toolName: string) {
  return toolName.replace(/__/g, ".");
}

function textOf(blocks: ContentBlock[]) {
  return blocks
    .filter((b) => b.type === "text" && !b.context && b.text)
    .map((b) => b.text!.trim())
    .filter(Boolean)
    .join("\n\n");
}

/** 工具结果的内容可能是字符串，也可能是 content block 数组。 */
export function resultText(content: unknown): string {
  if (content == null) return "";
  if (typeof content === "string") return content;
  if (Array.isArray(content))
    return content
      .map((c) =>
        c && typeof c === "object" && "text" in c
          ? String((c as { text: unknown }).text)
          : JSON.stringify(c),
      )
      .join("\n");
  return JSON.stringify(content, null, 2);
}

/*
 * 把消息整理成界面上的一条条：用户说的话、助手说的话、每次动作。
 * 动作的状态看后面有没有对应的 tool_result，再看待确认列表。
 */
export function buildTimeline(
  messages: Message[],
  pending: PendingAction[],
  tools: Tool[],
  opts: { running: boolean; streaming?: string },
): TimelineItem[] {
  const toolByName = new Map(tools.map((t) => [t.action, t]));
  const results = new Map<string, ContentBlock>();
  for (const m of messages)
    for (const b of m.content)
      if (b.type === "tool_result" && b.tool_use_id)
        results.set(b.tool_use_id, b);
  const pendingByUse = new Map(pending.map((p) => [p.toolUseId, p]));

  const items: TimelineItem[] = [];
  const sorted = [...messages].sort((a, b) => a.seq - b.seq);
  for (const m of sorted) {
    if (m.role === "user") {
      const text = textOf(m.content);
      if (text) items.push({ kind: "user", key: `m${m.id}`, text });
      continue;
    }
    // 助手的一条消息里，文字和工具调用按原来的顺序排。
    let buffer: string[] = [];
    const flush = (n: number) => {
      const text = buffer.join("\n\n").trim();
      if (text) items.push({ kind: "assistant", key: `m${m.id}-${n}`, text });
      buffer = [];
    };
    m.content.forEach((b, n) => {
      if (b.type === "text" && b.text) buffer.push(b.text);
      if (b.type !== "tool_use" || !b.id || !b.name) return;
      flush(n);
      const action = actionName(b.name);
      const tool = toolByName.get(action);
      const result = results.get(b.id);
      const p = pendingByUse.get(b.id);
      let status: ActionStatus = "running";
      if (p?.status === "pending") status = "waiting";
      else if (p?.status === "rejected") status = "rejected";
      else if (result) status = result.is_error ? "error" : "ok";
      else if (p?.status === "failed") status = "error";
      else if (p?.status === "done") status = "ok";
      else if (!opts.running) status = "error";
      items.push({
        kind: "action",
        key: `a${b.id}`,
        action,
        title: tool?.title ?? action,
        effect: tool?.effect ?? "unknown",
        input: (b.input ?? {}) as Record<string, unknown>,
        status,
        result: result ? resultText(result.content) : p?.result,
        pending: p?.status === "pending" ? p : undefined,
      });
    });
    flush(m.content.length);
  }
  if (opts.streaming?.trim())
    items.push({
      kind: "assistant",
      key: "streaming",
      text: opts.streaming,
      streaming: true,
    });
  return items;
}

/** 参数摘要：前三个字段，值太长就截断。 */
export function summarizeInput(
  input: Record<string, unknown>,
  max = 3,
): string {
  const parts = Object.entries(input)
    .filter(([, v]) => v !== undefined && v !== null && v !== "")
    .slice(0, max)
    .map(([k, v]) => {
      let s = typeof v === "string" ? v : JSON.stringify(v);
      s = s.replace(/\s+/g, " ");
      if (s.length > 28) s = `${s.slice(0, 27)}…`;
      return `${k}: ${s}`;
    });
  const rest = Object.keys(input).length - max;
  return parts.join(" · ") + (rest > 0 ? ` · +${rest}` : "");
}

/** 历史对话的标题，空的叫“新对话”。 */
export function conversationTitle(title: string) {
  return title.trim() || "新对话";
}
