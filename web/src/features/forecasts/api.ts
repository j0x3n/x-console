import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/forecasts";

/* 预测和决策记录（B137）。 */
export const forecastsApi = createApi<paths>();

type S = components["schemas"];
export type Forecast = S["Forecast"];
export type ForecastKind = S["ForecastKind"];
export type ForecastOutcome = S["ForecastOutcome"];
export type ForecastInput = S["ForecastInput"];
export type ForecastPatch = S["ForecastPatch"];
export type ForecastStats = S["ForecastStats"];
export type ForecastBin = S["ForecastBin"];
export type ForecastSettings = S["ForecastSettings"];

export const forecastKeys = {
  all: ["forecasts"] as const,
  list: ["forecasts", "list"] as const,
  stats: ["forecasts", "stats"] as const,
  settings: ["forecasts", "settings"] as const,
};

export function useForecasts() {
  return useQuery({
    queryKey: forecastKeys.list,
    queryFn: () => unwrap(forecastsApi.GET("/forecasts")),
    retry: false,
  });
}

export function useForecastStats() {
  return useQuery({
    queryKey: forecastKeys.stats,
    queryFn: () => unwrap(forecastsApi.GET("/forecasts/stats")),
    retry: false,
  });
}

export function useForecastSettings() {
  return useQuery({
    queryKey: forecastKeys.settings,
    queryFn: () => unwrap(forecastsApi.GET("/forecasts/settings")),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: forecastKeys.all });
}

export function useCreateForecast() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: ForecastInput) =>
      unwrap(forecastsApi.POST("/forecasts", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateForecast() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: ForecastPatch }) =>
      unwrap(
        forecastsApi.PATCH("/forecasts/{forecastId}", {
          params: { path: { forecastId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useResolveForecast() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; outcome: ForecastOutcome; note?: string }) =>
      unwrap(
        forecastsApi.POST("/forecasts/{forecastId}/resolve", {
          params: { path: { forecastId: v.id } },
          body: { outcome: v.outcome, note: v.note },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useReopenForecast() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        forecastsApi.POST("/forecasts/{forecastId}/reopen", {
          params: { path: { forecastId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteForecast() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        forecastsApi.DELETE("/forecasts/{forecastId}", {
          params: { path: { forecastId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function usePutForecastSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: ForecastSettings) =>
      unwrap(forecastsApi.PUT("/forecasts/settings", { body })),
    onSuccess: refresh,
  });
}
