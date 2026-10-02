import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ApiError, apiFetch, createApi, unwrap } from "../../api/client";
import { coreApi } from "../../api/core";
import { withElevation } from "../../auth/elevation";
import { invalidateOn, type ServerEvent } from "../../api/events";
import { queryClient } from "../../api/query";
import type { components, paths } from "../../api/gen/hosts";
import { appendPoint, pointFromSample, withSample } from "./lib";

export const hostsApi = createApi<paths>();

type S = components["schemas"];
export type Host = S["HostListItem"];
export type HostDetail = S["HostDetail"];
export type HostKind = S["HostKind"];
export type MetricsSample = S["MetricsSample"];
export type MetricsPoint = S["MetricsPoint"];
export type HostTraffic = S["HostTraffic"];
export type TrafficPlan = S["TrafficPlan"];
export type TrafficCountMode = S["TrafficCountMode"];
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
export type HostInfo = S["HostInfo"];
export type HostInfoInput = S["HostInfoInput"];
export type HostPatch = S["HostPatch"];
export type HostAddress = S["HostAddress"];

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
  traffic: (id: string, cycle: "current" | "previous") =>
    ["hosts", "traffic", id, cycle] as const,
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
// B82：信息和排序改了，列表和详情一起刷新。
invalidateOn("host.info.", hostsKeys.lists);
invalidateOn("host.info.", hostsKeys.details);
invalidateOn("host.order.", hostsKeys.lists);

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

export function useProcesses(id: string, sort: ProcessSort, limit = 300) {
  return useQuery({
    queryKey: [...hostsKeys.processes(id), sort, limit],
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/processes", {
          params: { path: { hostId: id }, query: { sort, limit } },
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
/** 读远端文件的一段（B33 日志查看）。旧后端和旧代理回 404 或 501。 */
export async function readFileRange(
  hostId: string,
  path: string,
  offset: number,
  length: number,
): Promise<Uint8Array> {
  const q = new URLSearchParams({
    path,
    offset: String(offset),
    length: String(length),
  });
  const res = await apiFetch(
    `/hosts/${encodeURIComponent(hostId)}/files/range?${q}`,
    { cache: "no-store" },
  );
  return new Uint8Array(await res.arrayBuffer());
}

export function followFilePath(hostId: string, path: string, offset: number) {
  const q = new URLSearchParams({ path, offset: String(offset) });
  return `/hosts/${encodeURIComponent(hostId)}/files/follow?${q}`;
}

export function downloadUrl(hostId: string, path: string) {
  return `/api/v1/hosts/${encodeURIComponent(hostId)}/files/content?path=${encodeURIComponent(path)}`;
}

/** 流量统计（B27）。接口还没上线时回 501，卡片显示“还没上线”。 */
export function useHostTraffic(id: string, cycle: "current" | "previous") {
  return useQuery({
    queryKey: hostsKeys.traffic(id, cycle),
    queryFn: () =>
      unwrap(
        hostsApi.GET("/hosts/{hostId}/traffic", {
          params: { path: { hostId: id }, query: { cycle } },
        }),
      ),
    retry: false,
    staleTime: 60_000,
    refetchInterval: cycle === "current" ? 60_000 : false,
  });
}

export function useSaveTrafficPlan(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (plan: TrafficPlan) =>
      unwrap(
        hostsApi.PUT("/hosts/{hostId}/traffic/plan", {
          params: { path: { hostId: id } },
          body: plan,
        }),
      ),
    onSuccess: (data) => {
      qc.setQueryData(hostsKeys.traffic(id, "current"), data);
      qc.invalidateQueries({ queryKey: ["hosts", "traffic", id, "previous"] });
      qc.invalidateQueries({ queryKey: hostsKeys.lists });
    },
  });
}

/* ---------- B82：机器信息、排序 ---------- */

/** 改名称和信息。改密码要提升权限，所以统一包 withElevation。 */
export function usePatchHost(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: HostPatch) => {
      const run = () =>
        unwrap(
          hostsApi.PATCH("/hosts/{hostId}", {
            params: { path: { hostId: id } },
            body,
          }),
        );
      return body.info?.password != null || body.info?.clearPassword
        ? withElevation(run)
        : run();
    },
    onSuccess: (data) => {
      qc.setQueryData(hostsKeys.detail(id), data);
      void qc.invalidateQueries({ queryKey: hostsKeys.lists });
    },
  });
}

/** 看登录密码：要提升权限，服务端写审计。 */
export function fetchHostPassword(id: string) {
  return withElevation(() =>
    unwrap(
      hostsApi.GET("/hosts/{hostId}/password", {
        params: { path: { hostId: id } },
      }),
    ).then((r) => r.password),
  );
}

/** 按给的顺序排。先改缓存，失败了再刷新回来。 */
export function useHostOrder(kind: HostKind) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) =>
      unwrap(hostsApi.PUT("/hosts/order", { body: { kind, ids } })),
    onMutate: (ids) => {
      qc.setQueryData<Host[]>(hostsKeys.list(kind), (old) =>
        old ? sortByIds(old, ids) : old,
      );
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: hostsKeys.lists }),
  });
}

/** 按 ids 的顺序重排，没列出的放最后。 */
export function sortByIds<T extends { id: string }>(list: T[], ids: string[]) {
  const pos = new Map(ids.map((id, i) => [id, i]));
  return [...list].sort(
    (a, b) => (pos.get(a.id) ?? Infinity) - (pos.get(b.id) ?? Infinity),
  );
}

/** 把 from 挪到 to 的位置，返回新的 id 顺序。 */
export function moveId(ids: string[], from: string, to: string): string[] {
  if (from === to || !ids.includes(from)) return ids;
  const rest = ids.filter((id) => id !== from);
  const at = rest.indexOf(to);
  if (at < 0) return ids;
  const fromIndex = ids.indexOf(from);
  const toIndex = ids.indexOf(to);
  rest.splice(fromIndex < toIndex ? at + 1 : at, 0, from);
  return rest;
}

/** 国家代码转国旗 emoji：JP → 🇯🇵。 */
export function flagEmoji(code: string): string {
  if (!/^[A-Za-z]{2}$/.test(code)) return "";
  return String.fromCodePoint(
    ...code
      .toUpperCase()
      .split("")
      .map((c) => 0x1f1e6 + c.charCodeAt(0) - 65),
  );
}
