import {
  Cloud,
  CloudDrizzle,
  CloudFog,
  CloudHail,
  CloudLightning,
  CloudMoon,
  CloudRain,
  CloudSnow,
  CloudSun,
  Cloudy,
  Haze,
  Moon,
  Snowflake,
  Sun,
  Thermometer,
  type LucideIcon,
} from "lucide-react";

/*
 * B90：天气对应的图标。天气条和左栏“今日”共用。
 * 有和风天气的图标代码（Weather.icon）时按它选，没有时按 WMO 代码（weatherCode）。
 */

export type WeatherKind =
  | "clear"
  | "partly"
  | "cloudy"
  | "overcast"
  | "fog"
  | "haze"
  | "drizzle"
  | "rain"
  | "storm"
  | "hail"
  | "sleet"
  | "snow"
  | "hot"
  | "unknown";

/** WMO 天气代码（Open-Meteo）。 */
export function kindFromWmo(code: number): WeatherKind {
  if (code === 0) return "clear";
  if (code === 1 || code === 2) return "partly";
  if (code === 3) return "overcast";
  if (code === 45 || code === 48) return "fog";
  if (code >= 51 && code <= 57) return "drizzle";
  if ((code >= 61 && code <= 67) || (code >= 80 && code <= 82)) return "rain";
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return "snow";
  if (code === 95) return "storm";
  if (code === 96 || code === 99) return "hail";
  return "unknown";
}

/** 和风天气的图标代码，见 https://dev.qweather.com/docs/resource/icons/ */
export function kindFromQWeather(icon: string): WeatherKind {
  const n = Number(icon);
  if (!Number.isFinite(n)) return "unknown";
  if (n === 100 || n === 150) return "clear";
  if ((n >= 101 && n <= 103) || (n >= 151 && n <= 153)) return "partly";
  if (n === 104) return "overcast";
  if (n === 304) return "hail";
  if (n >= 302 && n <= 303) return "storm";
  if (n === 309) return "drizzle";
  if (n === 313) return "sleet";
  if (n >= 300 && n <= 399) return "rain";
  if (n >= 404 && n <= 406) return "sleet";
  if (n >= 456 && n <= 457) return "sleet";
  if (n >= 400 && n <= 499) return "snow";
  if (n === 500 || n === 501 || (n >= 509 && n <= 515)) return "fog";
  if (n >= 502 && n <= 508) return "haze";
  if (n === 900) return "hot";
  return "unknown";
}

export function weatherKind(w: {
  weatherCode: number;
  icon?: string;
}): WeatherKind {
  if (w.icon) {
    const k = kindFromQWeather(w.icon);
    if (k !== "unknown") return k;
  }
  return kindFromWmo(w.weatherCode);
}

/** 没有 isDay 时按本地时间猜：6 点到 18 点算白天。 */
function daytime(isDay: boolean | undefined, now: Date) {
  if (isDay !== undefined) return isDay;
  const h = now.getHours();
  return h >= 6 && h < 18;
}

export function weatherIcon(
  w: { weatherCode: number; icon?: string; isDay?: boolean },
  now = new Date(),
): LucideIcon {
  const day = daytime(w.isDay, now);
  switch (weatherKind(w)) {
    case "clear":
      return day ? Sun : Moon;
    case "partly":
      return day ? CloudSun : CloudMoon;
    case "cloudy":
      return Cloud;
    case "overcast":
      return Cloudy;
    case "fog":
      return CloudFog;
    case "haze":
      return Haze;
    case "drizzle":
      return CloudDrizzle;
    case "rain":
      return CloudRain;
    case "storm":
      return CloudLightning;
    case "hail":
      return CloudHail;
    case "sleet":
      return CloudSnow;
    case "snow":
      return Snowflake;
    case "hot":
      return Thermometer;
    default:
      return CloudSun;
  }
}
