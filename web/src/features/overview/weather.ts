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
}

/** 分钟降水里最早有降水的时间。两小时内都没有时为 null。 */
export function rainSoon(
  minutely: WeatherExtra["minutely"],
  now: Date,
): RainSoon | null {
  const hit = minutely?.points.find((p) => p.precip > 0);
  if (!hit) return null;
  const inMinutes = Math.max(
    0,
    Math.round((Date.parse(hit.time) - now.getTime()) / 60_000),
  );
  return { kind: hit.kind === "snow" ? "snow" : "rain", inMinutes };
}

/** 天气条上的一小段字：“正在下雨”“20 分钟后有雪”。 */
export function rainSoonText(r: RainSoon): string {
  const what = r.kind === "snow" ? "雪" : "雨";
  return r.inMinutes <= 5 ? `正在下${what}` : `${r.inMinutes} 分钟后有${what}`;
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
