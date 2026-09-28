import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ApiError, apiFetch, createApi, unwrap } from "../../api/client";
import { coreApi } from "../../api/core";
import { invalidateOn, type ServerEvent } from "../../api/events";
import { queryClient } from "../../api/query";
import type { components, paths } from "../../api/gen/hosts";
import { appendPoint, pointFromSample, withSample } from "./lib";

export const hostsApi = createApi<paths>();

type S = components["schemas"];
export type Host = S["Host"];
export type HostDetail = S["HostDetail"];
export type HostKind = S["HostKind"];
export type MetricsSample = S["MetricsSample"];
export type MetricsPoint = S["MetricsPoint"];
export type MetricsSeries = S["MetricsSeries"];
export type MetricsRange = "1h" | "24h" | "7d";
export type Process = S["Process"];
export type Service = S["Service"];
export type ServiceAction = S["ServiceAction"];
export type FileEntry = S["FileEntry"];
export type FileList = S["FileList"];
export type ExecResult = S["ExecResult"];
export type PowerAction = S["PowerAction"];
export type SshHost = S["SshHost"];
export type SshHostInput = S["SshHostInput"];
export type AlertRule = S["AlertRule"];
export type AlertRuleInput = S["AlertRuleInput"];
export type AlertEvent = S["AlertEvent"];
export type ProcessSort = "cpu" | "mem" | "pid" | "name";

export const hostsKeys = {
  all: ["hosts"] as const,
  lists: ["hosts", "list"] as const,
  list: (kind?: HostKind) => ["hosts", "list", kind ?? "all"] as const,
  details: ["hosts", "detail"] as const,
  detail: (id: string) => ["hosts", "detail", id] as const,
  metrics: (id: string, range: MetricsRange) =>
    ["hosts", "metrics", id, range] as const,
  processes: (id: string) => ["hosts", "processes", id] as const,
  services: (id: string) => ["hosts", "services", id] as const,
  logs: (id: string, name: string) => ["hosts", "logs", id, name] as const,
  files: (id: string) => ["hosts", "files", id] as const,
  alerts: ["hosts", "alerts"] as const,
  rules: ["hosts", "rules"] as const,
  ssh: ["hosts", "ssh"] as const,
};

// 代理上线、下线，SSH 主机增删，告警变化时刷新列表和详情。
invalidateOn("agent.", hostsKeys.lists);
invalidateOn("agent.", hostsKeys.details);
invalidateOn("host.ssh.", hostsKeys.lists);
invalidateOn("host.ssh.", hostsKeys.ssh);
invalidateOn("host.alert", hostsKeys.alerts);
invalidateOn("host.alert", hostsKeys.rules);
invalidateOn("host.alert.", hostsKeys.lists);
invalidateOn("host.process.", ["hosts", "processes"]);
invalidateOn("host.service.", ["hosts", "services"]);

interface MetricsEvent {
  hostId: string;
  sample: MetricsSample;
}

/**
 * host.metrics 事件直接写进缓存：列表、详情、1 小时曲线。
 * 固定引用，可以直接交给 useServerEvent。
 */
export function applyMetricsEvent(event: ServerEvent) {
  const { hostId, sample } = event.data as MetricsEvent;
  if (!hostId || !sample) return;
  queryClient.setQueriesData<Host[]>({ queryKey: hostsKeys.lists }, (old) =>
    old?.map((h) => (h.id === hostId ? withSample(h, sample) : h)),
  );
  queryClient.setQueryData<HostDetail>(hostsKeys.detail(hostId), (old) =>
    old ? withSample(old, sample) : old,
  );
  queryClient.setQueryData<MetricsSeries>(
    hostsKeys.metrics(hostId, "1h"),
    (old) => appendPoint(old, pointFromSample(sample)),
  );
}

export function useHosts(kind?: HostKind) {
  return useQuery({
    queryKey: hostsKeys.list(kind),
    queryFn: () =>
      unwrap(hostsApi.GET("/hosts", { params: { query: { kind } } })),
  });
}

export function useHost(id: string) {
  return useQuery({
    queryKey: hostsKeys.detail(id),
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}", { params: { path: { hostId: id } } }),
      ),
  });
}

export function useHostMetrics(id: string, range: MetricsRange) {
  return useQuery({
    queryKey: hostsKeys.metrics(id, range),
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/metrics", {
          params: { path: { hostId: id }, query: { range } },
        }),
      ),
    placeholderData: keepPreviousData,
    // 1 小时曲线靠实时事件追加；24 小时和 7 天每分钟才变一次。
    staleTime: range === "1h" ? Infinity : 60_000,
  });
}

export function useProcesses(id: string, sort: ProcessSort) {
  return useQuery({
    queryKey: [...hostsKeys.processes(id), sort],
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/processes", {
          params: { path: { hostId: id }, query: { sort, limit: 300 } },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}

export function useServices(id: string, enabled = true) {
  return useQuery({
    queryKey: hostsKeys.services(id),
    enabled,
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/services", {
          params: { path: { hostId: id } },
        }),
      ),
  });
}

export function useServiceLogs(id: string, name: string | null) {
  return useQuery({
    queryKey: hostsKeys.logs(id, name ?? ""),
    enabled: !!name,
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/services/{name}/logs", {
          params: { path: { hostId: id, name: name! }, query: { lines: 300 } },
        }),
      ),
  });
}

export function useFiles(id: string, path: string) {
  return useQuery({
    queryKey: [...hostsKeys.files(id), path],
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/files", {
          params: { path: { hostId: id }, query: { path } },
        }),
      ),
    placeholderData: keepPreviousData,
    retry: false,
  });
}

export function useAlerts(hostId?: string) {
  return useQuery({
    queryKey: [...hostsKeys.alerts, hostId ?? "all"],
    queryFn: () =>
      unwrap(
        hostsApi.GET("/alerts", {
          params: { query: { hostId, limit: 100 } },
        }),
      ),
  });
}

export function useAlertRules() {
  return useQuery({
    queryKey: hostsKeys.rules,
    queryFn: () => unwrap(hostsApi.GET("/alert-rules")),
  });
}

export function useSshHosts() {
  return useQuery({
    queryKey: hostsKeys.ssh,
    queryFn: () => unwrap(hostsApi.GET("/ssh-hosts")),
  });
}

/** 刷新某台机器的某类数据的 mutation 帮手。 */
export function useHostMutation<TVars, TData = unknown>(
  fn: (vars: TVars) => Promise<TData>,
  invalidate: readonly (readonly unknown[])[],
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () =>
      invalidate.forEach((key) => qc.invalidateQueries({ queryKey: key })),
  });
}

/**
 * 打开终端前确认 5 分钟内验证过。浏览器看不到 WebSocket 握手的 403，
 * 所以先查一次登录状态；没提升就抛 elevation_required，交给 withElevation 弹框。
 */
export async function ensureElevated(): Promise<void> {
  const status = await unwrap(coreApi.GET("/auth/status"));
  const until = status.elevatedUntil
    ? new Date(status.elevatedUntil).getTime()
    : 0;
  if (until - Date.now() < 10_000) {
    throw new ApiError(403, "elevation_required", "需要再次验证");
  }
}

/** 上传文件（PUT 原始内容）。 */
export async function uploadFile(hostId: string, path: string, file: File) {
  const response = await apiFetch(
    `/hosts/${encodeURIComponent(hostId)}/files/content?path=${encodeURIComponent(path)}`,
    {
      method: "PUT",
      body: file,
      headers: { "Content-Type": "application/octet-stream" },
    },
  );
  return (await response.json()) as FileEntry;
}

/** 下载地址。GET 请求带 Cookie 就行。 */
export function downloadUrl(hostId: string, path: string) {
  return `/api/v1/hosts/${encodeURIComponent(hostId)}/files/content?path=${encodeURIComponent(path)}`;
}
