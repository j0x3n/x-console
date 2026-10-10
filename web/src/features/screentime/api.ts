import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/screentime";
import { withElevation } from "../../auth/elevation";

/* 电脑时间去向（B116）：Windows 代理每分钟记一次前台程序。 */
export const screentimeApi = createApi<paths>();

type S = components["schemas"];
export type ScreenSummary = S["ScreenTimeSummary"];
export type ScreenCategory = S["ScreenCategory"];
export type ScreenSettings = S["ScreenTimeSettings"];
export type ScreenSettingsInput = S["ScreenTimeSettingsInput"];
export type ScreenRule = S["ScreenTimeRule"];
export type ScreenRuleInput = S["ScreenTimeRuleInput"];
export type ScreenRange = "day" | "week" | "month";

export const screentimeKeys = {
  all: ["screentime"] as const,
  summary: (range: ScreenRange, date: string, host: string) =>
    ["screentime", "summary", range, date, host] as const,
  settings: ["screentime", "settings"] as const,
  rules: ["screentime", "rules"] as const,
};

/** 数据每分钟来一条，页面开着时每分钟刷新一次。 */
const REFRESH = 60_000;

export function useScreenSummary(
  range: ScreenRange,
  date: string,
  host: string,
) {
  return useQuery({
    queryKey: screentimeKeys.summary(range, date, host),
    queryFn: () =>
      unwrap(
        screentimeApi.GET("/screentime/summary", {
          params: {
            query: {
              range,
              ...(date ? { date } : {}),
              ...(host ? { hostId: host } : {}),
            },
          },
        }),
      ),
    refetchInterval: REFRESH,
    retry: false,
  });
}

export function useScreenSettings(enabled = true) {
  return useQuery({
    queryKey: screentimeKeys.settings,
    queryFn: () => unwrap(screentimeApi.GET("/screentime/settings")),
    enabled,
    retry: false,
  });
}

export function useSaveScreenSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ScreenSettingsInput) =>
      unwrap(screentimeApi.PUT("/screentime/settings", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: screentimeKeys.all }),
  });
}

export function useScreenRules(enabled = true) {
  return useQuery({
    queryKey: screentimeKeys.rules,
    queryFn: async () =>
      (await unwrap(screentimeApi.GET("/screentime/rules"))).items,
    enabled,
    retry: false,
  });
}

export function useCreateScreenRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ScreenRuleInput) =>
      unwrap(screentimeApi.POST("/screentime/rules", { body })),
    // 加规则会让已存的记录换类别，汇总也要重新拿
    onSuccess: () => qc.invalidateQueries({ queryKey: screentimeKeys.all }),
  });
}

export function useDeleteScreenRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        screentimeApi.DELETE("/screentime/rules/{ruleId}", {
          params: { path: { ruleId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: screentimeKeys.all }),
  });
}

export function useClearScreenData() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(screentimeApi.DELETE("/screentime/data"))),
    onSuccess: () => qc.invalidateQueries({ queryKey: screentimeKeys.all }),
  });
}
