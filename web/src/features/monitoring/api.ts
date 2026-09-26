import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/monitoring";
import type { paths as hostPaths } from "../../api/gen/hosts";

export const monitoringApi = createApi<paths>();
const hostsApi = createApi<hostPaths>();

type S = components["schemas"];
export type Monitor = S["Monitor"];
export type MonitorKind = S["MonitorKind"];
export type MonitorInput = S["MonitorInput"];
export type MonitorPatch = S["MonitorPatch"];
export type MonitorResult = S["MonitorResult"];
export type MonitorResults = S["MonitorResults"];
export type ResultRange = "24h" | "7d" | "30d";
export type Script = S["Script"];
export type ScriptInput = S["ScriptInput"];
export type ScriptPatch = S["ScriptPatch"];
export type ScriptShell = S["ScriptShell"];
export type ScriptRun = S["ScriptRun"];
export type Subscription = S["Subscription"];
export type SubscriptionInput = S["SubscriptionInput"];
export type SubscriptionPatch = S["SubscriptionPatch"];
export type SubscriptionEvent = S["SubscriptionEvent"];
export type SubscriptionSummary = S["SubscriptionSummary"];
export type SubscriptionCycle = S["SubscriptionCycle"];
export type SubscriptionCategory = S["SubscriptionCategory"];
export type DockerContainer = S["DockerContainer"];
export type DockerStats = S["DockerStats"];
export type DockerImage = S["DockerImage"];
export type ContainerAction = S["DockerContainerAction"];

export const monitoringKeys = {
  all: ["monitoring"] as const,
  monitors: ["monitoring", "monitors"] as const,
  results: (id: number, range: ResultRange) =>
    ["monitoring", "monitors", id, "results", range] as const,
  scripts: ["monitoring", "scripts"] as const,
  runs: (id: number) => ["monitoring", "scripts", id, "runs"] as const,
  subscriptions: ["monitoring", "subscriptions"] as const,
  subscriptionList: (archived: boolean) =>
    ["monitoring", "subscriptions", "list", archived] as const,
  summary: ["monitoring", "subscriptions", "summary"] as const,
  events: (id: number) => ["monitoring", "subscriptions", id, "events"] as const,
  hosts: ["monitoring", "hosts"] as const,
  docker: (hostId: string) => ["monitoring", "docker", hostId] as const,
};

// 服务端的变化都会发事件，收到就刷新对应的查询。
invalidateOn("monitor.", monitoringKeys.monitors);
invalidateOn("script.", monitoringKeys.scripts);
invalidateOn("script_run.", monitoringKeys.scripts);
invalidateOn("subscription.", monitoringKeys.subscriptions);
invalidateOn("agent.", monitoringKeys.hosts);
invalidateOn("docker.", ["monitoring", "docker"]);

function useInvalidate(key: readonly unknown[]) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: key });
}

// ---- 监控 ----

export function useMonitors() {
  return useQuery({
    queryKey: monitoringKeys.monitors,
    queryFn: () => unwrap(monitoringApi.GET("/monitors")),
  });
}

export function useMonitorResults(id: number | null, range: ResultRange) {
  return useQuery({
    queryKey: monitoringKeys.results(id ?? 0, range),
    queryFn: () =>
      unwrap(
        monitoringApi.GET("/monitors/{monitorId}/results", {
          params: { path: { monitorId: id ?? 0 }, query: { range } },
        }),
      ),
    enabled: id !== null,
  });
}

export function useSaveMonitor() {
  const invalidate = useInvalidate(monitoringKeys.monitors);
  return useMutation({
    mutationFn: ({ id, create, patch }: { id?: number; create?: MonitorInput; patch?: MonitorPatch }) =>
      id
        ? unwrap(
            monitoringApi.PATCH("/monitors/{monitorId}", {
              params: { path: { monitorId: id } },
              body: patch ?? {},
            }),
          )
        : unwrap(monitoringApi.POST("/monitors", { body: create! })),
    onSuccess: invalidate,
  });
}

export function useCheckMonitor() {
  const invalidate = useInvalidate(monitoringKeys.monitors);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        monitoringApi.POST("/monitors/{monitorId}/check", {
          params: { path: { monitorId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteMonitor() {
  const invalidate = useInvalidate(monitoringKeys.monitors);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        monitoringApi.DELETE("/monitors/{monitorId}", {
          params: { path: { monitorId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

// ---- 脚本 ----

export function useScripts() {
  return useQuery({
    queryKey: monitoringKeys.scripts,
    queryFn: () => unwrap(monitoringApi.GET("/scripts")),
  });
}

export function useScriptRuns(id: number | null) {
  return useQuery({
    queryKey: monitoringKeys.runs(id ?? 0),
    queryFn: () =>
      unwrap(
        monitoringApi.GET("/scripts/{scriptId}/runs", {
          params: { path: { scriptId: id ?? 0 }, query: { limit: 30 } },
        }),
      ),
    enabled: id !== null,
  });
}

export function useSaveScript() {
  const invalidate = useInvalidate(monitoringKeys.scripts);
  return useMutation({
    mutationFn: ({ id, create, patch }: { id?: number; create?: ScriptInput; patch?: ScriptPatch }) =>
      id
        ? unwrap(
            monitoringApi.PATCH("/scripts/{scriptId}", {
              params: { path: { scriptId: id } },
              body: patch ?? {},
            }),
          )
        : unwrap(monitoringApi.POST("/scripts", { body: create! })),
    onSuccess: invalidate,
  });
}

export function useDeleteScript() {
  const invalidate = useInvalidate(monitoringKeys.scripts);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        monitoringApi.DELETE("/scripts/{scriptId}", {
          params: { path: { scriptId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useRunScript() {
  const invalidate = useInvalidate(monitoringKeys.scripts);
  return useMutation({
    mutationFn: ({ id, hostIds }: { id: number; hostIds: string[] }) =>
      unwrap(
        monitoringApi.POST("/scripts/{scriptId}/run", {
          params: { path: { scriptId: id } },
          body: { hostIds },
        }),
      ),
    onSuccess: invalidate,
  });
}

/** 机器列表，给脚本选择执行的机器。 */
export function useHostOptions() {
  return useQuery({
    queryKey: monitoringKeys.hosts,
    queryFn: () => unwrap(hostsApi.GET("/hosts")),
    select: (hosts) =>
      hosts.map((h) => ({ id: h.id, name: h.name, online: h.online, kind: h.kind })),
  });
}

// ---- 订阅 ----

export function useSubscriptions(archived: boolean) {
  return useQuery({
    queryKey: monitoringKeys.subscriptionList(archived),
    queryFn: () =>
      unwrap(monitoringApi.GET("/subscriptions", { params: { query: { archived } } })),
  });
}

export function useSubscriptionSummary() {
  return useQuery({
    queryKey: monitoringKeys.summary,
    queryFn: () => unwrap(monitoringApi.GET("/subscriptions/summary")),
  });
}

export function useSubscriptionEvents(id: number | null) {
  return useQuery({
    queryKey: monitoringKeys.events(id ?? 0),
    queryFn: () =>
      unwrap(
        monitoringApi.GET("/subscriptions/{subscriptionId}/events", {
          params: { path: { subscriptionId: id ?? 0 } },
        }),
      ),
    enabled: id !== null,
  });
}

export function useSaveSubscription() {
  const invalidate = useInvalidate(monitoringKeys.subscriptions);
  return useMutation({
    mutationFn: ({
      id,
      create,
      patch,
    }: {
      id?: number;
      create?: SubscriptionInput;
      patch?: SubscriptionPatch;
    }) =>
      id
        ? unwrap(
            monitoringApi.PATCH("/subscriptions/{subscriptionId}", {
              params: { path: { subscriptionId: id } },
              body: patch ?? {},
            }),
          )
        : unwrap(monitoringApi.POST("/subscriptions", { body: create! })),
    onSuccess: invalidate,
  });
}

export function useDeleteSubscription() {
  const invalidate = useInvalidate(monitoringKeys.subscriptions);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        monitoringApi.DELETE("/subscriptions/{subscriptionId}", {
          params: { path: { subscriptionId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

// ---- Docker ----

export function useContainers(hostId: string, all: boolean) {
  return useQuery({
    queryKey: [...monitoringKeys.docker(hostId), "containers", all],
    queryFn: () =>
      unwrap(
        monitoringApi.GET("/hosts/{hostId}/docker/containers", {
          params: { path: { hostId }, query: { all } },
        }),
      ),
  });
}

export function useDockerStats(hostId: string) {
  return useQuery({
    queryKey: [...monitoringKeys.docker(hostId), "stats"],
    queryFn: () =>
      unwrap(monitoringApi.GET("/hosts/{hostId}/docker/stats", { params: { path: { hostId } } })),
  });
}

export function useDockerImages(hostId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...monitoringKeys.docker(hostId), "images"],
    queryFn: () =>
      unwrap(monitoringApi.GET("/hosts/{hostId}/docker/images", { params: { path: { hostId } } })),
    enabled,
  });
}

export function useContainerAction(hostId: string) {
  const invalidate = useInvalidate(monitoringKeys.docker(hostId));
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: ContainerAction }) =>
      unwrap(
        monitoringApi.POST("/hosts/{hostId}/docker/containers/{containerId}/{action}", {
          params: { path: { hostId, containerId: id, action } },
        }),
      ),
    onSuccess: invalidate,
  });
}
