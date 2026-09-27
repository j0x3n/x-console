import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/automations";

export const automationsApi = createApi<paths>();
type Schemas = components["schemas"];
export type Automation = Schemas["Automation"];
export type AutomationInput = Schemas["AutomationInput"];
export type Trigger = Schemas["Trigger"];
export type Condition = Schemas["Condition"];
export type Step = Schemas["Step"];
export type ActionCatalogItem = Schemas["ActionCatalogItem"];

export const automationKeys = {
  all: ["automations"] as const,
  list: ["automations", "list"] as const,
  catalog: ["automations", "catalog"] as const,
  runs: (id: string) => ["automations", id, "runs"] as const,
};
invalidateOn("automation.", automationKeys.all);

export function useAutomations() {
  return useQuery({
    queryKey: automationKeys.list,
    queryFn: () => unwrap(automationsApi.GET("/automations")),
  });
}
export function useAutomationCatalog() {
  return useQuery({
    queryKey: automationKeys.catalog,
    queryFn: () => unwrap(automationsApi.GET("/automations/catalog")),
  });
}
export function useAutomationRuns(id: string) {
  return useQuery({
    queryKey: automationKeys.runs(id),
    queryFn: () =>
      unwrap(
        automationsApi.GET("/automations/{id}/runs", {
          params: { path: { id } },
        }),
      ),
    enabled: !!id,
  });
}
export function useSaveAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id?: string; body: AutomationInput }) =>
      id
        ? unwrap(
            automationsApi.PUT("/automations/{id}", {
              params: { path: { id } },
              body,
            }),
          )
        : unwrap(automationsApi.POST("/automations", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: automationKeys.all }),
  });
}
export function useDeleteAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(
        automationsApi.DELETE("/automations/{id}", {
          params: { path: { id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: automationKeys.all }),
  });
}
export function useRunAutomation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(
        automationsApi.POST("/automations/{id}/run", {
          params: { path: { id } },
        }),
      ),
    onSuccess: (_, id) =>
      qc.invalidateQueries({ queryKey: automationKeys.runs(id) }),
  });
}
