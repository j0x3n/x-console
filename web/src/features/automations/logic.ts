import type {
  AutomationInput,
  CatalogAction,
  Condition,
  Step,
  Trigger,
} from "./api";

export const TRIGGER_TYPES: { type: Trigger["type"]; label: string }[] = [
  { type: "schedule", label: "定时" },
  { type: "event", label: "事件" },
  { type: "metric", label: "服务器指标" },
  { type: "ha_state", label: "智能家居状态" },
  { type: "webhook", label: "Webhook" },
];

export const CONDITION_OPS: Condition["op"][] = [
  "==",
  "!=",
  ">",
  ">=",
  "<",
  "<=",
  "contains",
];

const METRIC_LABELS: Record<string, string> = {
  cpu: "CPU",
  mem: "内存",
  disk: "磁盘",
};

export function emptyAutomation(): AutomationInput {
  return {
    name: "",
    enabled: true,
    trigger: { type: "schedule", cron: "0 9 * * *" },
    conditions: [],
    actions: [],
    cooldownSeconds: 0,
  };
}

/** 换触发器类型时给一组默认值。 */
export function defaultTrigger(type: Trigger["type"]): Trigger {
  switch (type) {
    case "schedule":
      return { type, cron: "0 9 * * *" };
    case "event":
      return { type, topic: "" };
    case "metric":
      return { type, metric: "cpu", op: ">", value: 90 };
    case "ha_state":
      return { type, entityId: "" };
    case "webhook":
      return { type };
  }
}

/** 列表里一句话说清触发器。 */
export function triggerSummary(t: Trigger): string {
  switch (t.type) {
    case "schedule":
      return `定时 ${t.cron ?? ""}`.trim();
    case "event":
      return `事件 ${t.topic || "（未填）"}${t.match ? `，${t.match}` : ""}`;
    case "metric":
      return `${t.hostId ? `${t.hostId} 的` : "任意服务器"} ${METRIC_LABELS[t.metric ?? "cpu"]} ${t.op ?? ">"} ${t.value ?? ""}%`;
    case "ha_state":
      return `${t.entityId || "（未填）"} 变成 ${t.to || "任何状态"}`;
    case "webhook":
      return t.webhookPath
        ? `Webhook ${t.webhookPath}`
        : "Webhook（保存后生成地址）";
  }
}

/** 保存前的检查，返回错误列表。 */
export function validateAutomation(a: AutomationInput): string[] {
  const errors: string[] = [];
  if (!a.name.trim()) errors.push("填一个名称");
  const t = a.trigger;
  if (t.type === "schedule" && (t.cron ?? "").trim().split(/\s+/).length !== 5)
    errors.push("cron 表达式要有 5 段，例如 0 9 * * 1-5");
  if (t.type === "event" && !t.topic?.trim()) errors.push("填要订阅的事件主题");
  if (t.type === "ha_state" && !t.entityId?.trim()) errors.push("填实体 id");
  if (t.type === "metric" && (t.value == null || Number.isNaN(t.value)))
    errors.push("填指标的阈值");
  a.conditions.forEach((c, i) => {
    if (!c.field.trim()) errors.push(`第 ${i + 1} 个条件没有填字段`);
  });
  if (a.actions.length === 0) errors.push("至少加一个动作");
  a.actions.forEach((s, i) => {
    if (!s.action) errors.push(`第 ${i + 1} 个动作没有选`);
  });
  if (a.cooldownSeconds < 0) errors.push("冷却时间不能是负数");
  return errors;
}

/** 规则里有没有 dangerous 动作。有的话保存时要提升权限。 */
export function hasDangerous(steps: Step[], catalog: CatalogAction[]): boolean {
  const effects = new Map(catalog.map((a) => [a.name, a.effect]));
  return steps.some((s) => effects.get(s.action) === "dangerous");
}

/*
 * 把动作的 JSON Schema 转成简单表单的字段。
 * 只支持一层对象，属性是字符串、数字、整数、布尔或枚举。
 * 有别的类型（数组、嵌套对象）时返回 null，界面退回 JSON 编辑框。
 */
export type FieldKind =
  | "string"
  | "text"
  | "number"
  | "integer"
  | "boolean"
  | "enum";

export interface Field {
  name: string;
  kind: FieldKind;
  label: string;
  required: boolean;
  options?: string[];
}

interface JsonSchema {
  type?: string | string[];
  properties?: Record<string, JsonSchema>;
  required?: string[];
  enum?: unknown[];
  description?: string;
  title?: string;
  format?: string;
  maxLength?: number;
}

const LONG_TEXT =
  /^(body|text|content|prompt|message|description|script|command)$/i;

export function schemaFields(schema: unknown): Field[] | null {
  const s = (schema ?? {}) as JsonSchema;
  if (s.type && s.type !== "object") return null;
  const props = s.properties ?? {};
  const required = new Set(s.required ?? []);
  const fields: Field[] = [];
  for (const [name, p] of Object.entries(props)) {
    const type = Array.isArray(p.type)
      ? p.type.find((x) => x !== "null")
      : p.type;
    const label = p.title ?? name;
    const base = { name, label, required: required.has(name) };
    if (p.enum && p.enum.every((v) => typeof v === "string"))
      fields.push({ ...base, kind: "enum", options: p.enum as string[] });
    else if (type === "string")
      fields.push({
        ...base,
        kind:
          LONG_TEXT.test(name) && (p.maxLength ?? 1000) > 200
            ? "text"
            : "string",
      });
    else if (type === "number") fields.push({ ...base, kind: "number" });
    else if (type === "integer") fields.push({ ...base, kind: "integer" });
    else if (type === "boolean") fields.push({ ...base, kind: "boolean" });
    else return null;
  }
  return fields;
}

/** 表单里的值转回动作输入：空字符串去掉，数字字段转数字（模板 {{...}} 保留字符串）。 */
export function cleanInput(
  input: Record<string, unknown>,
  fields: Field[] | null,
): Record<string, unknown> {
  if (!fields) return input;
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    const v = input[f.name];
    if (v === undefined || v === "") continue;
    if (
      (f.kind === "number" || f.kind === "integer") &&
      typeof v === "string"
    ) {
      out[f.name] =
        /\{\{.*\}\}/.test(v) || Number.isNaN(Number(v)) ? v : Number(v);
    } else out[f.name] = v;
  }
  return out;
}

/** 秒数写成好读的样子。 */
export function formatCooldown(seconds: number): string {
  if (seconds <= 0) return "不限制";
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`;
  return `${seconds} 秒`;
}
