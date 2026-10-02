import type {
  WarningLevel,
  WeatherExtra,
  WeatherWarning,
} from "../calendar/api";

/*
 * B58：和风天气扩展信息的显示规则。纯函数，不依赖 React。
 */

export type Tone = "" | "info" | "warn" | "accent" | "danger" | "ok";

const levelOrder: WarningLevel[] = [
  "red",
  "orange",
  "yellow",
  "blue",
  "unknown",
];

/** 预警颜色对应的界面颜色：红、橙、黄、蓝。 */
export function warningTone(level: WarningLevel): Tone {
  return (
    (
      { red: "danger", orange: "accent", yellow: "warn", blue: "info" } as const
    )[level as "red"] ?? ""
  );
}

const levelNames: Record<WarningLevel, string> = {
  red: "红色",
  orange: "橙色",
  yellow: "黄色",
  blue: "蓝色",
  unknown: "",
};

/** 短名字，比如“暴雨黄色预警”。 */
export function warningLabel(w: Pick<WeatherWarning, "typeName" | "level">) {
  return `${w.typeName}${levelNames[w.level]}预警`;
}

/** 颜色重的在前，同色按发布时间新的在前。 */
export function sortWarnings(list: WeatherWarning[]): WeatherWarning[] {
  return [...list].sort(
    (a, b) =>
      levelOrder.indexOf(a.level) - levelOrder.indexOf(b.level) ||
      b.issuedAt.localeCompare(a.issuedAt),
  );
}

export interface RainSoon {
  kind: "rain" | "snow";
  /** 0 表示正在下 */
  inMinutes: number;
  /**
   * B90：正在下时，还要下多久（分钟）。两小时内不停时为 null。
   * 还没开始下时不返回。
   */
  endsIn?: number | null;
}

/** 分钟降水里最早有降水的时间。两小时内都没有时为 null。 */
export function rainSoon(
  minutely: WeatherExtra["minutely"],
  now: Date,
): RainSoon | null {
  const points = minutely?.points ?? [];
  const hitIndex = points.findIndex((p) => p.precip > 0);
  if (hitIndex < 0) return null;
  const hit = points[hitIndex];
  const minutesTo = (iso: string) =>
    Math.max(0, Math.round((Date.parse(iso) - now.getTime()) / 60_000));
  const inMinutes = minutesTo(hit.time);
  const kind = hit.kind === "snow" ? "snow" : "rain";
  if (inMinutes > 5) return { kind, inMinutes };
  // 正在下：找下一个没有降水的点
  const stop = points.slice(hitIndex + 1).find((p) => p.precip <= 0);
  return { kind, inMinutes, endsIn: stop ? minutesTo(stop.time) : null };
}

/** 天气条上的一小段字：“正在下雨，约 25 分钟后停”“20 分钟后有雪”。 */
export function rainSoonText(r: RainSoon): string {
  const what = r.kind === "snow" ? "雪" : "雨";
  if (r.inMinutes > 5) return `${r.inMinutes} 分钟后有${what}`;
  if (r.endsIn === null) return `正在下${what}，两小时内不会停`;
  if (r.endsIn !== undefined && r.endsIn <= 5) return `正在下${what}，马上就停`;
  if (r.endsIn !== undefined) return `正在下${what}，约 ${r.endsIn} 分钟后停`;
  return `正在下${what}`;
}

/** 空气质量 1、2 级绿色，3 级黄色，4 级以上红色。 */
export function airTone(level: number): Tone {
  if (level <= 2) return "ok";
  if (level === 3) return "warn";
  return "danger";
}

/** 和昨天比：“比昨天高 3°”。差不到 1 度时不显示。 */
export function compareYesterday(
  today: { high: number },
  yesterday: WeatherExtra["yesterday"],
): string {
  if (!yesterday) return "";
  const d = Math.round(today.high - yesterday.high);
  if (d === 0) return "和昨天差不多";
  return d > 0 ? `比昨天高 ${d}°` : `比昨天低 ${-d}°`;
}
