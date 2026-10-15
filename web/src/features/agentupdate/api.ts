import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/agentupdate";
import { withElevation } from "../../auth/elevation";

/* 代理自动更新（B233）：每台机器的版本、自动更新开关和更新状态。 */
export const agentUpdateApi = createApi<paths>();

type S = components["schemas"];
export type AgentUpdate = S["AgentUpdate"];
export type AgentUpdateList = S["AgentUpdateList"];

export const agentUpdateKeys = {
  list: ["agentupdate", "list"] as const,
};

// 开始更新、更新失败、代理换成新版本重新连上，服务端都会发这个事件
invalidateOn("agentupdate.", agentUpdateKeys.list);

export function useAgentUpdates() {
  return useQuery({
    queryKey: agentUpdateKeys.list,
    queryFn: () => unwrap(agentUpdateApi.GET("/agent-updates")),
    retry: false,
    meta: { silentError: true },
    // 有机器在更新时多看几次，事件漏了也能看到结果
    refetchInterval: (q) =>
      q.state.data?.items.some((i) => i.state === "updating") ? 5_000 : false,
  });
}

export function useUpdateAgentNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) =>
      withElevation(() =>
        unwrap(
          agentUpdateApi.POST("/agent-updates/{agentId}", {
            params: { path: { agentId } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: agentUpdateKeys.list }),
  });
}

export function useSetAutoUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, on }: { agentId: string; on: boolean }) => {
      const save = () =>
        unwrap(
          agentUpdateApi.PUT("/agent-updates/{agentId}/auto", {
            params: { path: { agentId } },
            body: { on },
          }),
        );
      // 打开以后面板能往这台机器推新程序，要二次验证；关掉不用
      return on ? withElevation(save) : save();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: agentUpdateKeys.list }),
  });
}
