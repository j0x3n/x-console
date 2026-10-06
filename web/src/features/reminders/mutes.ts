/* 静音规则的显示逻辑（B113），纯函数，不依赖 React。 */

export interface KindPreset {
  /** 通配写法，和通知路由一样 */
  pattern: string;
  /** 英文原文，界面里用 t() */
  label: string;
}

/** 添加规则时可选的通知类型。 */
export const KIND_PRESETS: KindPreset[] = [
  { pattern: "*", label: "All notifications" },
  { pattern: "mail.*", label: "Mail" },
  { pattern: "github.*", label: "Repositories" },
  { pattern: "reminder.*", label: "Reminders" },
  { pattern: "host.*", label: "Servers" },
  { pattern: "weather.*", label: "Weather" },
  { pattern: "router.*", label: "Router" },
  { pattern: "coding_task.*", label: "Agent tasks" },
  { pattern: "habit.*", label: "Habits" },
];

/** 已知的写法给英文原文（界面里用 t()），不认识的返回 null，直接显示原文。 */
export function kindLabel(pattern: string): string | null {
  if (pattern === "mail.new") return "New mail";
  return KIND_PRESETS.find((p) => p.pattern === pattern)?.label ?? null;
}

export const CUSTOM_KIND = "__custom__";

/** 通知类型的写法是否合法，和服务端一样。 */
export function validKindPattern(p: string): boolean {
  return /^[a-z0-9_.*-]{1,64}$/.test(p);
}

export const BELL_TARGET = "bell";

/** 范围 mail:3 里的邮箱编号，不是邮件范围时返回 null。 */
export function mailScopeId(scope: string): number | null {
  const m = /^mail:(\d+)$/.exec(scope);
  return m ? Number(m[1]) : null;
}

export function mailScope(id: number): string {
  return `mail:${id}`;
}

/** webpush:5 里的设备编号，不是设备目标时返回 null。 */
export function deviceId(target: string): number | null {
  const m = /^webpush:(\d+)$/.exec(target);
  return m ? Number(m[1]) : null;
}

export function deviceTarget(id: number): string {
  return `webpush:${id}`;
}

/** 勾选框的状态换成要静音的目标列表：没勾的就是静音。 */
export function mutedFromChecked(
  all: string[],
  checked: Set<string>,
): string[] {
  return all.filter((t) => !checked.has(t));
}

/** 两个目标列表是不是同一批（不看顺序）。 */
export function sameTargets(a: string[], b: string[]): boolean {
  return (
    a.length === b.length &&
    [...a].sort().join("\n") === [...b].sort().join("\n")
  );
}
