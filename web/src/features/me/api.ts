import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { withElevation } from "../../auth/elevation";
import type { components, paths } from "../../api/gen/me";

/* 我的状态（B162）：工作中、暂离、睡觉、外出、在家。 */
export const meApi = createApi<paths>();

type S = components["schemas"];
export type MeState = S["MeState"];
export type MeStateName = S["MeStateName"];
export type MeDuration = S["MeManualInput"]["duration"];
export type MeHistory = S["MeHistory"];
export type MeDay = S["MeDay"];
export type MeSegment = S["MeSegment"];
export type MeSettings = S["MeSettings"];
export type MeSettingsInput = S["MeSettingsInput"];
export type MeReportStatus = S["MeReportStatus"];
export type MeReportToken = S["MeReportToken"];

export const meKeys = {
  all: ["me"] as const,
  state: ["me", "state"] as const,
  history: (days: number) => ["me", "history", days] as const,
  settings: ["me", "settings"] as const,
  report: ["me", "report"] as const,
};

// 状态变了发 me.state，起床发 me.woke，页面跟着刷新
invalidateOn("me.", meKeys.all);

export function useMe() {
  return useQuery({
    queryKey: meKeys.state,
    queryFn: () => unwrap(meApi.GET("/me")),
    retry: false,
  });
}

export function useMeHistory(days: number, enabled = true) {
  return useQuery({
    queryKey: meKeys.history(days),
    queryFn: () =>
      unwrap(meApi.GET("/me/history", { params: { query: { days } } })),
    enabled,
    retry: false,
  });
}

export function useSetManual() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: { state: MeStateName; duration: MeDuration }) =>
      unwrap(meApi.PUT("/me/manual", { body })),
    onSuccess: (data) => {
      client.setQueryData(meKeys.state, data);
      client.invalidateQueries({ queryKey: meKeys.all });
    },
  });
}

export function useClearManual() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(meApi.DELETE("/me/manual")),
    onSuccess: (data) => {
      client.setQueryData(meKeys.state, data);
      client.invalidateQueries({ queryKey: meKeys.all });
    },
  });
}

export function useMeSettings() {
  return useQuery({
    queryKey: meKeys.settings,
    queryFn: () => unwrap(meApi.GET("/me/settings")),
    retry: false,
  });
}

export function useSaveMeSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: MeSettingsInput) =>
      unwrap(meApi.PUT("/me/settings", { body })),
    onSuccess: (data) => {
      client.setQueryData(meKeys.settings, data);
      client.invalidateQueries({ queryKey: meKeys.all });
    },
  });
}

export function useMeReport() {
  return useQuery({
    queryKey: meKeys.report,
    queryFn: () => unwrap(meApi.GET("/me/report")),
    retry: false,
  });
}

export function useCreateMeReportToken() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(meApi.POST("/me/report/token"))),
    onSuccess: () => client.invalidateQueries({ queryKey: meKeys.report }),
  });
}

export function useDeleteMeReport() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(meApi.DELETE("/me/report")),
    onSuccess: () => client.invalidateQueries({ queryKey: meKeys.report }),
  });
}
