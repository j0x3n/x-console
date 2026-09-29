import type { AiModel, ModelRef } from "../api";

/*
 * AI 设置页（B32）用的纯函数：模型规格的显示、搜索、分组。
 */

/** 常见的 OpenAI 兼容接口，新建供应商时可以直接选。 */
export const PROVIDER_PRESETS = [
  { name: "OpenAI", baseUrl: "https://api.openai.com/v1" },
  { name: "DeepSeek", baseUrl: "https://api.deepseek.com/v1" },
  { name: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1" },
  { name: "Ollama", baseUrl: "http://127.0.0.1:11434/v1" },
];

/** 上下文长度：131072 → 128K，400000 → 400K，1000000 → 1M。 */
export function formatContext(tokens?: number): string {
  if (!tokens) return "";
  if (tokens >= 1_000_000) {
    const m = tokens / 1_000_000;
    return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
  }
  // 能被 1024 整除的按 1024 算，其他按 1000 算，和各家文档的写法一致。
  const k = tokens % 1024 === 0 ? tokens / 1024 : tokens / 1000;
  return `${Math.round(k)}K`;
}

/** 每百万 token 的美元价格，小于 1 保留两位。 */
export function formatPrice(price?: number): string {
  if (price === undefined) return "";
  if (price === 0) return "$0";
  return price < 1 ? `$${price.toFixed(2)}` : `$${Number(price.toFixed(2))}`;
}

/** “输入 / 输出”价格，都没有时返回空。 */
export function priceText(model: AiModel): string {
  if (model.inputPrice === undefined && model.outputPrice === undefined)
    return "";
  return `${formatPrice(model.inputPrice) || "?"} / ${formatPrice(model.outputPrice) || "?"}`;
}

/** 搜索：模型 id 或名字包含关键字，不区分大小写。 */
export function filterModels(models: AiModel[], q: string): AiModel[] {
  const s = q.trim().toLowerCase();
  if (!s) return models;
  return models.filter(
    (m) =>
      m.id.toLowerCase().includes(s) ||
      (m.name ?? "").toLowerCase().includes(s),
  );
}

export const sameModel = (a: ModelRef | undefined, m: AiModel) =>
  !!a && a.providerId === m.providerId && a.model === m.id;

export function findModel(
  models: AiModel[],
  ref: ModelRef | undefined,
): AiModel | undefined {
  return ref ? models.find((m) => sameModel(ref, m)) : undefined;
}

/** Agent 模型必须支持工具调用。规格未知的允许选，由用户自己负责。 */
export const canUseTools = (m: AiModel) => m.toolCall !== false;

/** 本月，YYYY-MM。 */
export function currentMonth(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
}

/** token 数：12345 → 12.3K，1234567 → 1.23M。 */
export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

/** 美元费用，两位小数。 */
export const formatCost = (cost?: number) =>
  cost === undefined ? "" : `$${cost.toFixed(2)}`;
