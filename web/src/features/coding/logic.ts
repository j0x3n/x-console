import type { components } from "../../api/gen/coding";

type S = components["schemas"];
export type Task = S["Task"];
export type TaskStatus = S["TaskStatus"];
export type TaskEvent = S["TaskEvent"];

/* ---- 状态 ---- */

export const STATUS_LABELS: Record<TaskStatus, string> = {
  queued: "Queued",
  running: "Running",
  review: "Needs review",
  failed: "Failed",
  canceled: "Canceled",
  committed: "Committed",
  pushed: "Pushed",
  pr_opened: "PR opened",
  discarded: "Discarded",
};

export type Tone = "ok" | "warn" | "danger" | "info" | "accent" | "";

export const STATUS_TONES: Record<TaskStatus, Tone> = {
  queued: "",
  running: "info",
  review: "accent",
  failed: "danger",
  canceled: "",
  committed: "ok",
  pushed: "ok",
  pr_opened: "ok",
  discarded: "",
};

export function isActive(status: TaskStatus) {
  return status === "queued" || status === "running";
}

/** 列表上的筛选。 */
export type Filter = "all" | "active" | "review" | "done" | "failed";

export const FILTERS: Array<{ id: Filter; label: string; statuses?: TaskStatus[] }> = [
  { id: "all", label: "All" },
  { id: "active", label: "In progress", statuses: ["queued", "running"] },
  { id: "review", label: "Needs review", statuses: ["review"] },
  { id: "done", label: "Done", statuses: ["committed", "pushed", "pr_opened"] },
  { id: "failed", label: "Failed or canceled", statuses: ["failed", "canceled", "discarded"] },
];

export function filterTasks(tasks: Task[], filter: Filter): Task[] {
  const statuses = FILTERS.find((f) => f.id === filter)?.statuses;
  return statuses ? tasks.filter((t) => statuses.includes(t.status)) : tasks;
}

const ORDER: Partial<Record<TaskStatus, number>> = { running: 0, queued: 1 };

/** 运行中的在最前，然后是排队的（先来先跑），其余按创建时间倒序。 */
export function sortTasks(tasks: Task[]): Task[] {
  return [...tasks].sort((a, b) => {
    const oa = ORDER[a.status] ?? 2;
    const ob = ORDER[b.status] ?? 2;
    if (oa !== ob) return oa - ob;
    if (a.status === "queued") return a.id - b.id;
    return b.id - a.id;
  });
}

/** 每个状态下能做的操作。和后端的规则一致。 */
export function actionsFor(task: Pick<Task, "status">, prAvailable: boolean) {
  const s = task.status;
  return {
    cancel: isActive(s),
    diff: ["review", "failed", "committed", "pushed", "pr_opened"].includes(s),
    commit: s === "review" || s === "failed",
    push: s === "committed",
    pr: prAvailable && (s === "committed" || s === "pushed"),
    discard: ["review", "failed", "committed"].includes(s),
  };
}

/* ---- 输出 ---- */

/** 按 seq 合并新事件，去掉重复的。 */
export function mergeEvents(current: TaskEvent[], incoming: TaskEvent[]): TaskEvent[] {
  if (incoming.length === 0) return current;
  const last = current.length ? current[current.length - 1].seq : 0;
  const fresh = incoming.filter((e) => e.seq > last).sort((a, b) => a.seq - b.seq);
  return fresh.length ? [...current, ...fresh] : current;
}

/** 新事件和已有事件之间有没有缺口（漏了中间的批次）。 */
export function hasGap(current: TaskEvent[], incoming: TaskEvent[]): boolean {
  if (incoming.length === 0) return false;
  const last = current.length ? current[current.length - 1].seq : 0;
  const first = Math.min(...incoming.map((e) => e.seq));
  return first > last + 1;
}

export interface ToolCall {
  id: string;
  name: string;
  summary: string;
  input?: unknown;
  result?: string;
  isError?: boolean;
  done: boolean;
}

export type OutputBlock =
  | { type: "text"; seq: number; text: string; stderr: boolean }
  | { type: "status"; seq: number; code: string; text: string; data: Record<string, unknown> }
  | { type: "error"; seq: number; text: string }
  | { type: "done"; seq: number; exitCode?: number; reason: string }
  | { type: "tools"; seq: number; calls: ToolCall[] };

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

/**
 * 把事件整理成显示用的块。连续的工具调用合成一组，结果按 id 配到调用上，
 * 界面上默认折叠。
 */
export function buildBlocks(events: TaskEvent[]): OutputBlock[] {
  const blocks: OutputBlock[] = [];
  let group: Extract<OutputBlock, { type: "tools" }> | null = null;
  const byId = new Map<string, ToolCall>();
  for (const ev of events) {
    const data = (ev.data ?? {}) as Record<string, unknown>;
    if (ev.kind === "tool") {
      const id = str(data.id);
      if (data.result === true) {
        const call = byId.get(id);
        if (call) {
          call.result = ev.text;
          call.isError = data.isError === true;
          call.done = true;
          continue;
        }
      }
      if (!group) {
        group = { type: "tools", seq: ev.seq, calls: [] };
        blocks.push(group);
      }
      const call: ToolCall = {
        id: id || `seq-${ev.seq}`,
        name: str(data.name) || "Tool",
        summary: ev.text,
        input: data.input,
        done: false,
      };
      if (data.result === true) {
        call.result = ev.text;
        call.summary = str(data.name) || "Tool";
        call.isError = data.isError === true;
        call.done = true;
      }
      group.calls.push(call);
      if (id) byId.set(id, call);
      continue;
    }
    group = null;
    switch (ev.kind) {
      case "text":
        blocks.push({ type: "text", seq: ev.seq, text: ev.text, stderr: data.stream === "stderr" });
        break;
      case "status":
        blocks.push({ type: "status", seq: ev.seq, code: str(data.code), text: ev.text, data });
        break;
      case "error":
        blocks.push({ type: "error", seq: ev.seq, text: ev.text });
        break;
      case "done":
        blocks.push({ type: "done", seq: ev.seq, exitCode: ev.exitCode, reason: str(data.reason) || ev.text });
        break;
    }
  }
  return blocks;
}

/* ---- diff ---- */

export interface DiffLine {
  kind: "add" | "del" | "ctx" | "hunk" | "meta";
  text: string;
  oldNo?: number;
  newNo?: number;
}

export interface DiffFile {
  path: string;
  lines: DiffLine[];
  additions: number;
  deletions: number;
  binary: boolean;
}

/** 解析 git diff 的统一格式，按文件分开。 */
export function parseDiff(diff: string): DiffFile[] {
  const files: DiffFile[] = [];
  let file: DiffFile | null = null;
  let oldNo = 0;
  let newNo = 0;
  let inHunk = false;
  for (const raw of diff.split("\n")) {
    if (raw.startsWith("diff --git ")) {
      const m = /^diff --git a\/(.*) b\/(.*)$/.exec(raw);
      file = { path: m ? m[2] : raw.slice(11), lines: [], additions: 0, deletions: 0, binary: false };
      files.push(file);
      inHunk = false;
      continue;
    }
    if (!file) continue;
    if (raw.startsWith("@@")) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw);
      oldNo = m ? Number(m[1]) : 0;
      newNo = m ? Number(m[2]) : 0;
      inHunk = true;
      file.lines.push({ kind: "hunk", text: raw });
      continue;
    }
    if (!inHunk) {
      if (raw.startsWith("+++ b/")) file.path = raw.slice(6);
      if (raw.startsWith("Binary files")) file.binary = true;
      continue;
    }
    if (raw.startsWith("+")) {
      file.lines.push({ kind: "add", text: raw.slice(1), newNo: newNo++ });
      file.additions++;
    } else if (raw.startsWith("-")) {
      file.lines.push({ kind: "del", text: raw.slice(1), oldNo: oldNo++ });
      file.deletions++;
    } else if (raw.startsWith(" ")) {
      file.lines.push({ kind: "ctx", text: raw.slice(1), oldNo: oldNo++, newNo: newNo++ });
    } else if (raw.startsWith("\\")) {
      file.lines.push({ kind: "meta", text: raw });
    }
  }
  return files;
}

/** 需求里的第一行，列表上当标题用。 */
export function taskTitle(task: Pick<Task, "title" | "id">) {
  return task.title || `#${task.id}`;
}

export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}
