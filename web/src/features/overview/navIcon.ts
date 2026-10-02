import { useQuery } from "@tanstack/react-query";
import { unwrap } from "../../api/client";
import type { NavIconState } from "../../lib/navBadges";
import { briefApi, calendarKeys } from "../calendar/api";
import { weatherIcon } from "./weatherIcon";

/**
 * B88：左栏“今日”的图标跟着天气变，和今日页天气条用同一份数据。
 * 没设置天气或取不到时用原来的太阳（返回 null）。
 */
export function useTodayNavIcon(): NavIconState | null {
  const weather = useQuery({
    queryKey: calendarKeys.weather,
    queryFn: () => unwrap(briefApi.GET("/weather")),
    staleTime: 10 * 60_000,
    retry: false,
    meta: { silentError: true },
  });
  const w = weather.data;
  if (!w) return null;
  return {
    icon: weatherIcon(w),
    title: `${w.location ? `${w.location} ` : ""}${w.summary} ${Math.round(w.temperature)}°`,
  };
}
