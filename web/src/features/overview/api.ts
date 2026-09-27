import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/dashboard";
import { briefApi, calendarKeys } from "../calendar/api";
import { normalizeLayout, type LayoutCard } from "./layout";

export const dashboardApi = createApi<paths>();

export type DashboardLayout = components["schemas"]["DashboardLayout"];

export const overviewKeys = {
  all: ["overview"] as const,
  layout: ["overview", "layout"] as const,
};

export interface LayoutState {
  cards: LayoutCard[];
  /** 服务端还没有布局接口（404）。这时用默认布局，改动不能保存。 */
  unavailable: boolean;
}

export function isNotFound(error: unknown) {
  return error instanceof ApiError && error.status === 404;
}

export function useDashboardLayout() {
  return useQuery({
    queryKey: overviewKeys.layout,
    queryFn: async (): Promise<LayoutState> => {
      try {
        const data = await unwrap(dashboardApi.GET("/dashboard/layout"));
        return { cards: normalizeLayout(data.cards), unavailable: false };
      } catch (error) {
        if (isNotFound(error))
          return { cards: normalizeLayout([]), unavailable: true };
        throw error;
      }
    },
    staleTime: 60_000,
  });
}

export function useSaveLayout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cards: LayoutCard[]) =>
      unwrap(dashboardApi.PUT("/dashboard/layout", { body: { cards } })),
    onSuccess: (data) =>
      qc.setQueryData<LayoutState>(overviewKeys.layout, {
        cards: normalizeLayout(data.cards),
        unavailable: false,
      }),
  });
}

/** 天气。没设置位置时服务端返回 412，由卡片显示“去设置”。 */
export function useWeather() {
  return useQuery({
    queryKey: calendarKeys.weather,
    queryFn: () => unwrap(briefApi.GET("/weather")),
    staleTime: 10 * 60_000,
    retry: false,
  });
}
