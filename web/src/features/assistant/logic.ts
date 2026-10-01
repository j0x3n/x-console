import type { ContentBlock, Message, PendingAction, Tool } from "./api";

export type ActionStatus = "running" | "waiting" | "ok" | "error" | "rejected";

export interface MessageAttachment {
  id: number;
  name: string;
  kind: "image" | "file";
}

export type TimelineItem =
  | {
      kind: "user";
      key: string;
      text: string;
      attachments?: MessageAttachment[];
    }
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

/** B39：用户消息里的附件块。 */
function attachmentsOf(blocks: ContentBlock[]): MessageAttachment[] {
  return blocks
    .filter(
      (b) =>
        (b.type === "image" || b.type === "file") &&
        typeof b.attachmentId === "number",
    )
    .map((b) => ({
      id: b.attachmentId as number,
      name: String(b.name ?? ""),
      kind: b.type as "image" | "file",
    }));
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
      const attachments = attachmentsOf(m.content);
      if (text || attachments.length)
        items.push({
          kind: "user",
          key: `m${m.id}`,
          text,
          attachments: attachments.length ? attachments : undefined,
        });
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
        effect: p?.effect ?? tool?.effect ?? "unknown",
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

/*
 * 很长的结果只显示开头和结尾，中间写省略了多少行。
 * 超过 max 行时折叠，开头和结尾各留 max / 2 行。
 */
export function foldLines(
  text: string,
  max = 200,
): { text: string; hidden: number } {
  const lines = text.split("\n");
  if (lines.length <= max) return { text, hidden: 0 };
  const half = Math.floor(max / 2);
  const hidden = lines.length - half * 2;
  return {
    text: [
      ...lines.slice(0, half),
      `…… 省略 ${hidden} 行 ……`,
      ...lines.slice(-half),
    ].join("\n"),
    hidden,
  };
}

/*
 * 机器 Agent（B33）的工具。模型看到的名字是 host__run_command 这样，
 * 界面上换回 host.run_command。effect 是默认的风险，run_command 由后端按命令判断，
 * 放在待确认动作的 effect 里。
 */
export const HOST_TOOLS: Tool[] = [
  {
    name: "host__run_command",
    action: "host.run_command",
    title: "Run command",
    effect: "write",
  },
  {
    name: "host__read_file",
    action: "host.read_file",
    title: "Read file",
    effect: "read",
  },
  {
    name: "host__write_file",
    action: "host.write_file",
    title: "Write file",
    effect: "write",
  },
  {
    name: "host__list_dir",
    action: "host.list_dir",
    title: "List folder",
    effect: "read",
  },
  {
    name: "host__system_info",
    action: "host.system_info",
    title: "System info",
    effect: "read",
  },
  {
    name: "host__list_processes",
    action: "host.list_processes",
    title: "List processes",
    effect: "read",
  },
  {
    name: "host__list_services",
    action: "host.list_services",
    title: "List services",
    effect: "read",
  },
  {
    name: "host__list_containers",
    action: "host.list_containers",
    title: "List containers",
    effect: "read",
  },
  {
    name: "host__service_action",
    action: "host.service_action",
    title: "Service action",
    effect: "write",
  },
  {
    name: "host__container_action",
    action: "host.container_action",
    title: "Container action",
    effect: "write",
  },
];

/** AI 记下或改了一条记忆时，回复里显示“已记住：……”（B61）。 */
export function rememberedText(item: TimelineItem): string | null {
  if (item.kind !== "action" || item.status !== "ok") return null;
  if (item.action !== "memory.save" && item.action !== "memory.update")
    return null;
  return typeof item.input.text === "string" && item.input.text.trim()
    ? item.input.text.trim()
    : null;
}
