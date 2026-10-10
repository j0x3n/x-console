import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/aiconfig";
import { withElevation } from "../../auth/elevation";

/* AI 编码工具配置统一下发（B121）：面板里写一份，下发到所选机器。 */
export const aiconfigApi = createApi<paths>();

type S = components["schemas"];
export type AIConfigState = S["AIConfigState"];
export type AIConfigInput = S["AIConfigInput"];
export type AIConfigTool = S["AIConfigTool"];
export type AIConfigMcp = S["AIConfigMcp"];
export type AIConfigHost = S["AIConfigHost"];
export type AIConfigItem = S["AIConfigItem"];
export type AIConfigHostStatus = S["AIConfigHostStatus"];
export type AIConfigStatus = S["AIConfigStatus"];

export const aiconfigKeys = {
  all: ["aiconfig"] as const,
  config: ["aiconfig", "config"] as const,
  status: ["aiconfig", "status"] as const,
};

// 保存和下发各发一次事件，别的窗口也跟着刷新
invalidateOn("aiconfig.", aiconfigKeys.all);

export function useAIConfig() {
  return useQuery({
    queryKey: aiconfigKeys.config,
    queryFn: () => unwrap(aiconfigApi.GET("/aiconfig")),
    retry: false,
  });
}

/** 实时检查所有已选机器，只读。机器多时要几秒，所以单独一个查询。 */
export function useAIConfigStatus(enabled: boolean) {
  return useQuery({
    queryKey: aiconfigKeys.status,
    queryFn: () => unwrap(aiconfigApi.GET("/aiconfig/status")),
    enabled,
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
}

export function useSaveAIConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AIConfigInput) =>
      unwrap(aiconfigApi.PUT("/aiconfig", { body })),
    onSuccess: (data) => {
      qc.setQueryData(aiconfigKeys.config, data);
      return qc.invalidateQueries({ queryKey: aiconfigKeys.status });
    },
  });
}

/** 下发。要提升权限。hostIds 不传就是所有已选机器。 */
export function useApplyAIConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostIds?: string[]) =>
      withElevation(() =>
        unwrap(
          aiconfigApi.POST("/aiconfig/apply", {
            body: hostIds ? { hostIds } : {},
          }),
        ),
      ),
    onSuccess: (data) => {
      // 只下发了一部分机器时，别的机器保留原来的结果
      qc.setQueryData<AIConfigStatus>(aiconfigKeys.status, (old) => {
        if (!old) return data;
        const byId = new Map(data.hosts.map((h) => [h.hostId, h]));
        return {
          hosts: old.hosts.map((h) => byId.get(h.hostId) ?? h),
        };
      });
    },
  });
}
