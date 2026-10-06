import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/quotas";
import { withElevation } from "../../auth/elevation";

/* AI 额度（B110、B111）：Claude、Codex、Grok 的额度窗口，DeepSeek 的余额。 */
export const quotasApi = createApi<paths>();

type S = components["schemas"];
export type QuotaAccount = S["QuotaAccount"];
export type QuotaAccountInput = S["QuotaAccountInput"];
export type QuotaAccountPatch = S["QuotaAccountPatch"];
export type QuotaWindow = S["QuotaWindow"];
export type QuotaBalance = S["QuotaBalance"];
export type QuotaHost = S["QuotaHost"];
export type QuotaKind = S["QuotaKind"];

export const quotaKeys = {
  all: ["quotas"] as const,
  list: ["quotas", "list"] as const,
  hosts: ["quotas", "hosts"] as const,
};

// 服务端每读完一个账号发一次事件
invalidateOn("quota.", quotaKeys.list);

export function useQuotaAccounts() {
  return useQuery({
    queryKey: quotaKeys.list,
    queryFn: async () => (await unwrap(quotasApi.GET("/quotas"))).items,
    retry: false,
    // 事件漏了也不超过 1 分钟
    refetchInterval: 60_000,
  });
}

export function useQuotaHosts(enabled = true) {
  return useQuery({
    queryKey: quotaKeys.hosts,
    queryFn: async () => (await unwrap(quotasApi.GET("/quotas/hosts"))).items,
    enabled,
  });
}

export function useCreateQuotaAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: QuotaAccountInput) =>
      withElevation(() => unwrap(quotasApi.POST("/quotas", { body }))),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.list }),
  });
}

export function useUpdateQuotaAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: number; body: QuotaAccountPatch }) =>
      withElevation(() =>
        unwrap(
          quotasApi.PATCH("/quotas/{accountId}", {
            params: { path: { accountId: v.id } },
            body: v.body,
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.list }),
  });
}

export function useDeleteQuotaAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          quotasApi.DELETE("/quotas/{accountId}", {
            params: { path: { accountId: id } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.list }),
  });
}

/** 立即读一次。服务端 5 分钟内不重复读，失败的 30 秒后可以再读。 */
export function useRefreshQuotaAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        quotasApi.POST("/quotas/{accountId}/refresh", {
          params: { path: { accountId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.list }),
  });
}

export function useReorderQuotaAccounts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: number[]) =>
      unwrap(quotasApi.POST("/quotas/reorder", { body: { ids } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.list }),
  });
}
