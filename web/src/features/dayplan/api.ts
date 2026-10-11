import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/dayplan";

/* AI 排今天的计划（B135）。 */
export const dayplanApi = createApi<paths>();

type S = components["schemas"];
export type DayPlan = S["DayPlan"];
export type DayPlanItem = S["DayPlanItem"];
export type DayPlanItemInput = S["DayPlanItemInput"];
export type DayPlanSlot = S["DayPlanSlot"];
export type DayPlanSettings = S["DayPlanSettings"];

export const planKeys = {
  all: ["dayplan"] as const,
  day: (date: string) => ["dayplan", "day", date] as const,
  settings: ["dayplan", "settings"] as const,
};

// 服务端每次生成、修改、确认、勾选都发 dayplan.changed
invalidateOn("dayplan.", planKeys.all);

/** date 不给就是今天。 */
export function useDayPlan(date?: string) {
  return useQuery({
    queryKey: planKeys.day(date ?? ""),
    queryFn: () =>
      unwrap(
        dayplanApi.GET("/dayplan", {
          params: { query: date ? { date } : {} },
        }),
      ),
    retry: false,
  });
}

export function useDayPlanSettings() {
  return useQuery({
    queryKey: planKeys.settings,
    queryFn: () => unwrap(dayplanApi.GET("/dayplan/settings")),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: planKeys.all });
}

export function useGeneratePlan() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { date?: string; replace?: boolean }) =>
      unwrap(dayplanApi.POST("/dayplan/generate", { body: v })),
    onSuccess: refresh,
  });
}

export function useSavePlanItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { date: string; items: DayPlanItemInput[] }) =>
      unwrap(
        dayplanApi.PUT("/dayplan/{date}", {
          params: { path: { date: v.date } },
          body: { items: v.items },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useConfirmPlan() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (date: string) =>
      unwrap(
        dayplanApi.POST("/dayplan/{date}/confirm", {
          params: { path: { date } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useSetItemDone() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; done: boolean }) =>
      unwrap(
        dayplanApi.PUT("/dayplan/items/{itemId}/done", {
          params: { path: { itemId: v.id } },
          body: { done: v.done },
        }),
      ),
    onSuccess: refresh,
  });
}

export function usePutPlanSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: DayPlanSettings) =>
      unwrap(dayplanApi.PUT("/dayplan/settings", { body })),
    onSuccess: refresh,
  });
}

/** 把计划里的事项转成保存用的输入。 */
export function toInputs(items: DayPlanItem[]): DayPlanItemInput[] {
  return items.map((it) => ({
    title: it.title,
    kind: it.kind,
    ref: it.ref,
    start: it.start,
    end: it.end,
    reason: it.reason,
    done: it.done,
  }));
}
