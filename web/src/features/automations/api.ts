import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, errorMessage, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/automations";
import { withElevation } from "../../auth/elevation";
import { toast } from "../../hooks/useToast";

export const automationsApi = createApi<paths>();

export type Automation = components["schemas"]["Automation"];
export type AutomationInput = components["schemas"]["AutomationInput"];
export type Trigger = components["schemas"]["Trigger"];
export type Condition = components["schemas"]["Condition"];
export type Step = components["schemas"]["Step"];
export type Run = components["schemas"]["Run"];
export type Catalog = components["schemas"]["Catalog"];
export type CatalogAction = components["schemas"]["CatalogAction"];

export const automationKeys = {
  all: ["automations"] as const,
  list: ["automations", "list"] as const,
  item: (id: number) => ["automations", "item", id] as const,
  runs: (id: number) => ["automations", "runs", id] as const,
  catalog: ["automations", "catalog"] as const,
};

invalidateOn("automation.", automationKeys.all);

/** 服务端还没有自动化接口（404 或 501）。只用在列表和目录上判断。 */
export function isNotLive(error: unknown) {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  );
}

export function useAutomations() {
  return useQuery({
    queryKey: automationKeys.list,
    queryFn: () => unwrap(automationsApi.GET("/automations")),
  });
}

export function useAutomation(id: number | null) {
  return useQuery({
    queryKey: automationKeys.item(id ?? 0),
    queryFn: () =>
      unwrap(
        automationsApi.GET("/automations/{automationId}", {
          params: { path: { automationId: id! } },
        }),
      ),
    enabled: id != null,
  });
}

export function useRuns(id: number | null) {
  return useQuery({
    queryKey: automationKeys.runs(id ?? 0),
    queryFn: () =>
      unwrap(
        automationsApi.GET("/automations/{automationId}/runs", {
          params: { path: { automationId: id! }, query: { limit: 50 } },
        }),
      ),
    enabled: id != null,
  });
}

export function useCatalog() {
  return useQuery({
    queryKey: automationKeys.catalog,
    queryFn: () => unwrap(automationsApi.GET("/automations/catalog")),
    staleTime: 5 * 60_000,
  });
}

const fail = (error: unknown) =>
  toast({ message: errorMessage(error), tone: "error" });

/** 保存规则。有 dangerous 动作时服务端要求提升权限，withElevation 会弹验证框。 */
export function useSaveAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number | null; body: AutomationInput }) =>
      withElevation(() =>
        id == null
          ? unwrap(automationsApi.POST("/automations", { body }))
          : unwrap(
              automationsApi.PUT("/automations/{automationId}", {
                params: { path: { automationId: id } },
                body,
              }),
            ),
      ),
    onSuccess: (saved) => {
      qc.setQueryData(automationKeys.item(saved.id), saved);
      qc.invalidateQueries({ queryKey: automationKeys.list });
    },
  });
}

export function useToggleAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      unwrap(
        automationsApi.PATCH("/automations/{automationId}", {
          params: { path: { automationId: id } },
          body: { enabled },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: automationKeys.all }),
    onError: fail,
  });
}

export function useDeleteAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        automationsApi.DELETE("/automations/{automationId}", {
          params: { path: { automationId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: automationKeys.all }),
    onError: fail,
  });
}

export function useRunAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        automationsApi.POST("/automations/{automationId}/run", {
          params: { path: { automationId: id } },
        }),
      ),
    onSuccess: (_d, id) => {
      toast("已开始运行");
      qc.invalidateQueries({ queryKey: automationKeys.runs(id) });
    },
    onError: fail,
  });
}
