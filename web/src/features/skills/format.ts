/* 技能页用的纯函数。 */

/** 分钟数写成“1 小时 30 分钟”或“45 分钟”。 */
export function formatMinutes(n: number): string {
  if (n < 60) return `${n} 分钟`;
  const h = Math.floor(n / 60);
  const m = n % 60;
  return m === 0 ? `${h} 小时` : `${h} 小时 ${m} 分钟`;
}

/** 本周进度占目标的百分比，最多 100。没有目标时返回 null。 */
export function targetPercent(
  minutes: number,
  target?: number | null,
): number | null {
  if (!target || target <= 0) return null;
  return Math.min(100, Math.round((minutes / target) * 100));
}

/** 最近 8 周的标签：最后一项是“本周”，其余是几周前。 */
export function weekLabels(count: number): string[] {
  return Array.from({ length: count }, (_, i) =>
    i === count - 1 ? "0" : String(i - (count - 1)),
  );
}
