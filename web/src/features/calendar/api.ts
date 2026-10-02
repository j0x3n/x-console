import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { useInvalidate } from "../../api/useInvalidate";
import type {
  components as CalendarComponents,
  paths as CalendarPaths,
} from "../../api/gen/calendar";
import type {
  components as BriefComponents,
  paths as BriefPaths,
} from "../../api/gen/brief";
import type {
  components as FocusComponents,
  paths as FocusPaths,
} from "../../api/gen/focus";

/* M11 的三个后端模块：calendar、brief、focus。前端都放在 features/calendar。 */

export const calendarApi = createApi<CalendarPaths>();
export const briefApi = createApi<BriefPaths>();
export const focusApi = createApi<FocusPaths>();

export type Calendar = CalendarComponents["schemas"]["Calendar"];
export type CalendarInput = CalendarComponents["schemas"]["CalendarInput"];
export type CalendarPatch = CalendarComponents["schemas"]["CalendarPatch"];
export type CalendarEvent = CalendarComponents["schemas"]["CalendarEvent"];
export type EventInput = CalendarComponents["schemas"]["EventInput"];
export type EventPatch = CalendarComponents["schemas"]["EventPatch"];
export type Brief = BriefComponents["schemas"]["Brief"];
export type BriefSettings = BriefComponents["schemas"]["BriefSettings"];
export type BriefSettingsView = BriefComponents["schemas"]["BriefSettingsView"];
export type BriefSectionKey = BriefComponents["schemas"]["BriefSectionKey"];
export type Weather = BriefComponents["schemas"]["Weather"];
export type WeatherPlace = BriefComponents["schemas"]["WeatherPlace"];
// B58 和风天气
export type QWeatherConfig = BriefComponents["schemas"]["QWeatherConfig"];
export type QWeatherConfigInput =
  BriefComponents["schemas"]["QWeatherConfigInput"];
export type WeatherExtra = BriefComponents["schemas"]["WeatherExtra"];
export type WeatherWarning = BriefComponents["schemas"]["WeatherWarning"];
export type WarningLevel = BriefComponents["schemas"]["WarningLevel"];
export type WeatherNotify = BriefComponents["schemas"]["WeatherNotify"];
export type Earthquake = BriefComponents["schemas"]["Earthquake"];
export type BriefLocation = BriefComponents["schemas"]["BriefLocation"];
export type RainAlert = BriefComponents["schemas"]["RainAlert"];
export type FocusSession = FocusComponents["schemas"]["FocusSession"];
export type FocusStats = FocusComponents["schemas"]["FocusStats"];

export const calendarKeys = {
  all: ["calendar"] as const,
  calendars: ["calendar", "calendars"] as const,
  events: (from: string, to: string) =>
    ["calendar", "events", from, to] as const,
  eventsAll: ["calendar", "events"] as const,
  briefs: ["calendar", "briefs"] as const,
  brief: (date: string) => ["calendar", "briefs", "one", date] as const,
  briefSettings: ["calendar", "briefs", "settings"] as const,
  weather: ["calendar", "weather"] as const,
  focus: ["calendar", "focus"] as const,
  focusCurrent: ["calendar", "focus", "current"] as const,
  focusStats: (days: number) => ["calendar", "focus", "stats", days] as const,
};

invalidateOn("calendar.", calendarKeys.calendars);
invalidateOn("calendar.", calendarKeys.eventsAll);
invalidateOn("brief.", calendarKeys.briefs);
invalidateOn("focus.", calendarKeys.focus);

// ---- calendars ----

export function useCreateEvent() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: (body: EventInput) =>
      unwrap(calendarApi.POST("/calendar/events", { body })),
    onSuccess: invalidate,
  });
}

export function useUpdateEvent() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: EventPatch }) =>
      unwrap(
        calendarApi.PATCH("/calendar/events/{eventId}", {
          params: { path: { eventId: id } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteEvent() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        calendarApi.DELETE("/calendar/events/{eventId}", {
          params: { path: { eventId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useCalendars() {
  return useQuery({
    queryKey: calendarKeys.calendars,
    queryFn: () => unwrap(calendarApi.GET("/calendars")),
  });
}

export function useCalendarEvents(from: Date, to: Date) {
  const f = from.toISOString();
  const t = to.toISOString();
  return useQuery({
    queryKey: calendarKeys.events(f, t),
    queryFn: () =>
      unwrap(
        calendarApi.GET("/calendar/events", {
          params: { query: { from: f, to: t } },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}

export function useCreateCalendar() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: (body: CalendarInput) =>
      unwrap(calendarApi.POST("/calendars", { body })),
    onSuccess: invalidate,
  });
}

export function useUpdateCalendar() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: CalendarPatch }) =>
      unwrap(
        calendarApi.PATCH("/calendars/{calendarId}", {
          params: { path: { calendarId: id } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteCalendar() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        calendarApi.DELETE("/calendars/{calendarId}", {
          params: { path: { calendarId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useSyncCalendar() {
  const invalidate = useInvalidate(calendarKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        calendarApi.POST("/calendars/{calendarId}/sync", {
          params: { path: { calendarId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

// ---- briefs ----

export function useBriefs() {
  return useQuery({
    queryKey: calendarKeys.briefs,
    queryFn: () =>
      unwrap(briefApi.GET("/briefs", { params: { query: { limit: 60 } } })),
    select: (data) => data.items,
  });
}

export function useBrief(date: string | null) {
  return useQuery({
    queryKey: calendarKeys.brief(date ?? ""),
    queryFn: () =>
      unwrap(
        briefApi.GET("/briefs/{date}", { params: { path: { date: date! } } }),
      ),
    enabled: !!date,
  });
}

export function useGenerateBrief() {
  const invalidate = useInvalidate(calendarKeys.briefs);
  return useMutation({
    mutationFn: (send: boolean) =>
      unwrap(briefApi.POST("/briefs/generate", { body: { send } })),
    onSuccess: (_data, send) => {
      if (send) invalidate();
    },
  });
}

export function useBriefSettings() {
  return useQuery({
    queryKey: calendarKeys.briefSettings,
    queryFn: () => unwrap(briefApi.GET("/briefs/settings")),
  });
}

export function useSaveBriefSettings() {
  const invalidate = useInvalidate(calendarKeys.briefs);
  return useMutation({
    mutationFn: (body: BriefSettings) =>
      unwrap(briefApi.PUT("/briefs/settings", { body })),
    onSuccess: invalidate,
  });
}

export function useWeatherPlaces(q: string) {
  return useQuery({
    queryKey: ["calendar", "weather", "places", q],
    queryFn: () =>
      unwrap(briefApi.GET("/weather/places", { params: { query: { q } } })),
    enabled: q.trim().length > 0,
    staleTime: 60 * 60_000,
    retry: false,
  });
}

export async function resolveWeatherLocation(
  lat: number,
  lon: number,
): Promise<BriefLocation> {
  const q = `${lon.toFixed(2)},${lat.toFixed(2)}`;
  const places = await unwrap(
    briefApi.GET("/weather/places", { params: { query: { q } } }),
  );
  const city = places[0];
  if (!city) throw new Error("和风天气没有找到当前位置");
  return { id: city.id, lat: city.lat, lon: city.lon, name: city.name };
}

export function useRainAlert() {
  return useQuery({
    queryKey: ["calendar", "weather", "alert"],
    queryFn: () => unwrap(briefApi.GET("/weather/alert")),
    retry: false,
  });
}

export function useSaveRainAlert() {
  const invalidate = useInvalidate(calendarKeys.weather);
  return useMutation({
    mutationFn: (body: RainAlert) =>
      unwrap(briefApi.PUT("/weather/alert", { body })),
    onSuccess: invalidate,
  });
}

// ---- B58 和风天气、预警、地震 ----

export const weatherExtraKey = ["calendar", "weather", "extra"] as const;
// 服务端发现新的预警或地震时发 weather.updated
invalidateOn("weather.", weatherExtraKey);

export function useQWeatherConfig() {
  return useQuery({
    queryKey: ["calendar", "qweather"],
    queryFn: () => unwrap(briefApi.GET("/weather/qweather")),
    retry: false,
    meta: { silentError: true },
  });
}

export function useSaveQWeatherConfig() {
  const invalidate = useInvalidate(["calendar"]);
  return useMutation({
    mutationFn: (body: QWeatherConfigInput) =>
      unwrap(briefApi.PUT("/weather/qweather", { body })),
    onSuccess: invalidate,
  });
}

/** 预警、分钟降水、空气、指数、天文、昨天、地震。接口没上线时报 404 或 501。 */
export function useWeatherExtra() {
  return useQuery({
    queryKey: weatherExtraKey,
    queryFn: () => unwrap(briefApi.GET("/weather/extra")),
    staleTime: 5 * 60_000,
    retry: false,
    meta: { silentError: true },
  });
}

export function useWeatherNotify() {
  return useQuery({
    queryKey: ["calendar", "weather", "notify"],
    queryFn: () => unwrap(briefApi.GET("/weather/notify")),
    retry: false,
    meta: { silentError: true },
  });
}

export function useSaveWeatherNotify() {
  const invalidate = useInvalidate(calendarKeys.weather);
  return useMutation({
    mutationFn: (body: WeatherNotify) =>
      unwrap(briefApi.PUT("/weather/notify", { body })),
    onSuccess: invalidate,
  });
}

// ---- focus ----

export function useCurrentFocus() {
  return useQuery({
    queryKey: calendarKeys.focusCurrent,
    queryFn: () => unwrap(focusApi.GET("/focus/current")),
    select: (data) => data.session ?? null,
  });
}

export function useFocusStats(days: number) {
  return useQuery({
    queryKey: calendarKeys.focusStats(days),
    queryFn: () =>
      unwrap(focusApi.GET("/focus/stats", { params: { query: { days } } })),
  });
}

export function useStartFocus() {
  const invalidate = useInvalidate(calendarKeys.focus);
  return useMutation({
    mutationFn: (body: { minutes?: number; issueKey?: string }) =>
      unwrap(focusApi.POST("/focus/start", { body })),
    onSuccess: invalidate,
  });
}

export function useStopFocus() {
  const invalidate = useInvalidate(calendarKeys.focus);
  return useMutation({
    mutationFn: ({ id, completed }: { id: number; completed?: boolean }) =>
      unwrap(
        focusApi.POST("/focus/{focusId}/stop", {
          params: { path: { focusId: id } },
          body: completed === undefined ? {} : { completed },
        }),
      ),
    onSuccess: invalidate,
  });
}
