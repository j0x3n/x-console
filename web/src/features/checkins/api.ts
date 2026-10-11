import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/checkins";

/* 每天状态打卡（B132）。 */
export const checkinsApi = createApi<paths>();

type S = components["schemas"];
export type CheckinQuestion = S["CheckinQuestion"];
export type CheckinQuestionInput = S["CheckinQuestionInput"];
export type CheckinSlot = S["CheckinSlot"];
export type CheckinAnswerInput = S["CheckinAnswerInput"];
export type CheckinSeries = S["CheckinSeries"];

export const checkinKeys = {
  all: ["checkins"] as const,
  questions: ["checkins", "questions"] as const,
  settings: ["checkins", "settings"] as const,
  today: ["checkins", "today"] as const,
  series: (days: number) => ["checkins", "series", days] as const,
};

// 服务端每记一条回答发 checkin.answered
invalidateOn("checkin.", checkinKeys.all);

export function useCheckinQuestions() {
  return useQuery({
    queryKey: checkinKeys.questions,
    queryFn: () => unwrap(checkinsApi.GET("/checkins/questions")),
    retry: false,
  });
}

export function useCheckinSettings() {
  return useQuery({
    queryKey: checkinKeys.settings,
    queryFn: () => unwrap(checkinsApi.GET("/checkins/settings")),
    retry: false,
  });
}

export function useCheckinsToday() {
  return useQuery({
    queryKey: checkinKeys.today,
    queryFn: () => unwrap(checkinsApi.GET("/checkins/today")),
    retry: false,
    // 时间点的状态随时间变，开着页面时每分钟刷一次
    refetchInterval: 60_000,
  });
}

export function useCheckinSeries(days: number) {
  return useQuery({
    queryKey: checkinKeys.series(days),
    queryFn: () =>
      unwrap(
        checkinsApi.GET("/checkins/series", { params: { query: { days } } }),
      ),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: checkinKeys.all });
}

export function usePutCheckinSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (enabled: boolean) =>
      unwrap(checkinsApi.PUT("/checkins/settings", { body: { enabled } })),
    onSuccess: refresh,
  });
}

export function useAnswerCheckin() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: CheckinAnswerInput) =>
      unwrap(checkinsApi.PUT("/checkins/answers", { body })),
    onSuccess: refresh,
  });
}

export function useSaveCheckinQuestion() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id?: number; body: CheckinQuestionInput }) =>
      v.id == null
        ? unwrap(checkinsApi.POST("/checkins/questions", { body: v.body }))
        : unwrap(
            checkinsApi.PUT("/checkins/questions/{questionId}", {
              params: { path: { questionId: v.id } },
              body: v.body,
            }),
          ),
    onSuccess: refresh,
  });
}

export function useDeleteCheckinQuestion() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        checkinsApi.DELETE("/checkins/questions/{questionId}", {
          params: { path: { questionId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}
