import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/knowledge";

/* 个人资料问答（B139）。 */
export const knowledgeApi = createApi<paths>();

type S = components["schemas"];
export type KnowledgeHit = S["KnowledgeHit"];
export type KnowledgeAnswer = S["KnowledgeAnswer"];
export type KnowledgeStatus = S["KnowledgeStatus"];
export type KnowledgeSettings = S["KnowledgeSettings"];

export const knowledgeKeys = {
  all: ["knowledge"] as const,
  status: ["knowledge", "status"] as const,
  settings: ["knowledge", "settings"] as const,
};

/** 状态。正在同步时每 3 秒刷新一次。 */
export function useKnowledgeStatus() {
  return useQuery({
    queryKey: knowledgeKeys.status,
    queryFn: () => unwrap(knowledgeApi.GET("/knowledge/status")),
    refetchInterval: (query) => (query.state.data?.syncing ? 3000 : false),
    retry: false,
  });
}

export function useKnowledgeSettings() {
  return useQuery({
    queryKey: knowledgeKeys.settings,
    queryFn: () => unwrap(knowledgeApi.GET("/knowledge/settings")),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: knowledgeKeys.all });
}

export function useAskKnowledge() {
  return useMutation({
    mutationFn: (question: string) =>
      unwrap(knowledgeApi.POST("/knowledge/ask", { body: { question } })),
  });
}

export function useSyncKnowledge() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () => unwrap(knowledgeApi.POST("/knowledge/sync")),
    onSuccess: refresh,
  });
}

export function usePutKnowledgeSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: KnowledgeSettings) =>
      unwrap(knowledgeApi.PUT("/knowledge/settings", { body })),
    onSuccess: refresh,
  });
}

export function useClearKnowledge() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () => unwrap(knowledgeApi.DELETE("/knowledge/index")),
    onSuccess: refresh,
  });
}
