import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/experiments";

/* 自我实验（B133）。 */
export const experimentsApi = createApi<paths>();

type S = components["schemas"];
export type Experiment = S["Experiment"];
export type ExperimentInput = S["ExperimentInput"];
export type ExperimentDetail = S["ExperimentDetail"];
export type ExperimentResult = S["ExperimentResult"];
export type ExperimentMetricOption = S["ExperimentMetricOption"];
export type ExperimentPhase = S["ExperimentPhase"];
export type ExperimentVerdict = S["ExperimentVerdict"];

export const experimentKeys = {
  all: ["experiments"] as const,
  list: ["experiments", "list"] as const,
  metrics: ["experiments", "metrics"] as const,
  detail: (id: number) => ["experiments", "detail", id] as const,
};

export function useExperiments() {
  return useQuery({
    queryKey: experimentKeys.list,
    queryFn: () => unwrap(experimentsApi.GET("/experiments")),
    retry: false,
  });
}

export function useExperiment(id: number) {
  return useQuery({
    queryKey: experimentKeys.detail(id),
    queryFn: () =>
      unwrap(
        experimentsApi.GET("/experiments/{experimentId}", {
          params: { path: { experimentId: id } },
        }),
      ),
    retry: false,
  });
}

export function useExperimentMetrics(enabled = true) {
  return useQuery({
    queryKey: experimentKeys.metrics,
    queryFn: () => unwrap(experimentsApi.GET("/experiments/metrics")),
    enabled,
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: experimentKeys.all });
}

export function useSaveExperiment() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id?: number; body: ExperimentInput }) =>
      v.id == null
        ? unwrap(experimentsApi.POST("/experiments", { body: v.body }))
        : unwrap(
            experimentsApi.PUT("/experiments/{experimentId}", {
              params: { path: { experimentId: v.id } },
              body: v.body,
            }),
          ),
    onSuccess: refresh,
  });
}

export function useDeleteExperiment() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        experimentsApi.DELETE("/experiments/{experimentId}", {
          params: { path: { experimentId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 把一个实验转成保存用的输入，改动字段后再存。 */
export function toInput(e: Experiment): ExperimentInput {
  return {
    title: e.title,
    change: e.change,
    baselineStart: e.baselineStart,
    changeStart: e.changeStart,
    endDate: e.endDate,
    note: e.note,
    conclusion: e.conclusion,
    stopped: e.stopped,
    metrics: e.metrics.map((m) => m.key),
  };
}
