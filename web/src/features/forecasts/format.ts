import type { ForecastKind, ForecastOutcome } from "./api";

/** 三个结果按钮的文字：预测问“成没成”，决策问“对不对”。 */
export const OUTCOME_LABEL: Record<
  ForecastKind,
  Record<ForecastOutcome, string>
> = {
  prediction: { yes: "Came true", no: "Did not come true", void: "Void" },
  decision: { yes: "Was right", no: "Was wrong", void: "Unclear" },
};

/** 偏差的一句话说明。绝对值小于 5 个百分点算接近。 */
export function biasText(bias: number): "close" | "high" | "low" {
  if (Math.abs(bias) < 5) return "close";
  return bias > 0 ? "high" : "low";
}

/** 把握画成 0 到 100 的宽度。 */
export function pct(n: number): string {
  return `${Math.max(0, Math.min(100, n))}%`;
}
