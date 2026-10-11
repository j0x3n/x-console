import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/goals";

/* 目标和每周复盘（B128）。 */
export const goalsApi = createApi<paths>();

type S = components["schemas"];
export type Goal = S["Goal"];
export type GoalResult = S["GoalResult"];
export type GoalInput = S["GoalInput"];
export type GoalResultInput = S["GoalResultInput"];
export type GoalStatus = S["GoalStatus"];
export type GoalState = S["GoalState"];
export type GoalSource = S["GoalSource"];
export type GoalPeriod = S["GoalPeriod"];
export type GoalMetric = S["GoalMetric"];
export type GoalSettings = S["GoalSettings"];
export type GoalReview = S["GoalReview"];
export type GoalReviewInput = S["GoalReviewInput"];
export type GoalReviewSummary = S["GoalReviewSummary"];
export type GoalWeekGoal = S["GoalWeekGoal"];
export type GoalWeekResult = S["GoalWeekResult"];

export const goalKeys = {
  all: ["goals"] as const,
  list: (period?: GoalPeriod, status?: GoalStatus) =>
    [
      "goals",
      "list",
      period?.year ?? 0,
      period?.quarter ?? 0,
      status ?? "",
    ] as const,
  metrics: ["goals", "metrics"] as const,
  settings: ["goals", "settings"] as const,
  reviews: ["goals", "reviews"] as const,
  review: (week: string) => ["goals", "review", week] as const,
};

// 服务端每次增删改、更新进度、保存复盘都发 goal.* 事件
invalidateOn("goal.", goalKeys.all);

/** 一个季度的目标（不给季度就是全部）。状态筛选在服务端做。 */
export function useGoals(period?: GoalPeriod, status?: GoalStatus) {
  return useQuery({
    queryKey: goalKeys.list(period, status),
    queryFn: () =>
      unwrap(
        goalsApi.GET("/goals", {
          params: {
            query: {
              ...(period ? { year: period.year, quarter: period.quarter } : {}),
              ...(status ? { status } : {}),
            },
          },
        }),
      ),
    retry: false,
  });
}

/** 能绑定的数据来源和它们的选项。 */
export function useGoalMetrics(enabled = true) {
  return useQuery({
    queryKey: goalKeys.metrics,
    queryFn: () => unwrap(goalsApi.GET("/goals/metrics")),
    enabled,
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: goalKeys.all });
}

export function useSaveGoal() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id?: number; body: GoalInput }) =>
      v.id == null
        ? unwrap(goalsApi.POST("/goals", { body: v.body }))
        : unwrap(
            goalsApi.PUT("/goals/{goalId}", {
              params: { path: { goalId: v.id } },
              body: v.body,
            }),
          ),
    onSuccess: refresh,
  });
}

export function useSetGoalStatus() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; status: GoalStatus }) =>
      unwrap(
        goalsApi.PATCH("/goals/{goalId}", {
          params: { path: { goalId: v.id } },
          body: { status: v.status },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteGoal() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        goalsApi.DELETE("/goals/{goalId}", {
          params: { path: { goalId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 手动关键结果设当前值。 */
export function useSetResultValue() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; value: number }) =>
      unwrap(
        goalsApi.POST("/goals/results/{resultId}/value", {
          params: { path: { resultId: v.id } },
          body: { value: v.value },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useGoalSettings() {
  return useQuery({
    queryKey: goalKeys.settings,
    queryFn: () => unwrap(goalsApi.GET("/goals/settings")),
    retry: false,
  });
}

export function usePutGoalSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: GoalSettings) =>
      unwrap(goalsApi.PUT("/goals/settings", { body })),
    onSuccess: refresh,
  });
}

/** week 是 "current" 或 YYYY-MM-DD。 */
export function useGoalReview(week: string) {
  return useQuery({
    queryKey: goalKeys.review(week),
    queryFn: () =>
      unwrap(
        goalsApi.GET("/goals/reviews/{week}", { params: { path: { week } } }),
      ),
    retry: false,
  });
}

export function useGoalReviews() {
  return useQuery({
    queryKey: goalKeys.reviews,
    queryFn: () =>
      unwrap(goalsApi.GET("/goals/reviews", { params: { query: {} } })),
    retry: false,
  });
}

export function useSaveReview() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { week: string; body: GoalReviewInput }) =>
      unwrap(
        goalsApi.PUT("/goals/reviews/{week}", {
          params: { path: { week: v.week } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}
