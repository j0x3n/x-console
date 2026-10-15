import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/router";
import { withElevation } from "../../auth/elevation";
import { useRouterInterval } from "./interval";

/* OpenWrt 主路由（B65）。面板通过路由器的 ubus 读状态，路由器上不装代理。 */
export const routerApi = createApi<paths>();

type S = components["schemas"];
export type RouterConfig = S["RouterConfig"];
export type RouterConfigInput = S["RouterConfigInput"];
export type RouterMode = S["RouterMode"];
export type RouterPushToken = S["RouterPushToken"];
export type RouterStatus = S["RouterStatus"];
export type RouterInterface = S["RouterInterface"];
export type RouterClient = S["RouterClient"];
export type RouterTraffic = S["RouterTraffic"];
export type TrafficRange = "24h" | "7d";
export type RouterDevice = S["RouterDevice"];
export type RouterOptions = S["RouterOptions"];
/** 再次通知间隔可选的分钟数，和后端一致（B228）。 */
export const COOLDOWNS = [0, 5, 15, 30, 60] as const;

/** 再次通知间隔的文字，0 表示每次都推。 */
export function cooldownLabel(minutes: number) {
  return minutes === 0 ? "每次都推" : `${minutes} 分钟`;
}

export const routerKeys = {
  all: ["router"] as const,
  config: ["router", "config"] as const,
  status: ["router", "status"] as const,
  clients: ["router", "clients"] as const,
  traffic: (range: TrafficRange) => ["router", "traffic", range] as const,
  devices: ["router", "devices"] as const,
  options: ["router", "options"] as const,
};

/** 路径里的 MAC 不带冒号，省得转义。后端认冒号、横杠和不带分隔的写法。 */
export const macPath = (mac: string) => mac.replace(/[:-]/g, "");

/** 不是主动上报模式，没有设备列表（B228）。 */
export function isNotPush(error: unknown) {
  return error instanceof ApiError && error.code === "router_not_push";
}

/** 还没填路由器地址。 */
export function isNotConfigured(error: unknown) {
  return (
    error instanceof ApiError && error.code === "integration_not_configured"
  );
}

export function useRouterConfig() {
  return useQuery({
    queryKey: routerKeys.config,
    queryFn: () => unwrap(routerApi.GET("/router/config")),
    retry: false,
  });
}

/**
 * 路由器状态。速率按两次读数算，刷新越快越接近实时。
 * B93：刷新频率用路由器页的设置（useRouterInterval），路由器页、左栏和今日页一样。
 * 传了 intervalMs 就用它（0 表示不自动刷新）。
 * 连不上时用到的地方都在页面里显示错误，不再弹提示。
 */
export function useRouterStatus(intervalMs?: number) {
  const chosen = useRouterInterval();
  const every = intervalMs ?? chosen;
  return useQuery({
    queryKey: routerKeys.status,
    queryFn: () => unwrap(routerApi.GET("/router/status")),
    retry: false,
    meta: { silentError: true },
    // 连不上时 30 秒再试一次，路由器重启完会自己恢复；没配置时不再请求
    refetchInterval: (q) =>
      q.state.status !== "error"
        ? every || false
        : isNotConfigured(q.state.error)
          ? false
          : 30_000,
  });
}

/** 在线设备：跟着刷新频率，但最快 5 秒，少打扰路由器（B93）。 */
export function useRouterClients(enabled = true) {
  const chosen = useRouterInterval();
  return useQuery({
    queryKey: routerKeys.clients,
    queryFn: () => unwrap(routerApi.GET("/router/clients")),
    retry: false,
    enabled,
    meta: { silentError: true },
    refetchInterval: chosen === 0 ? false : Math.max(chosen, 5000),
  });
}

export function useRouterTraffic(range: TrafficRange, enabled = true) {
  return useQuery({
    queryKey: routerKeys.traffic(range),
    queryFn: () =>
      unwrap(
        routerApi.GET("/router/traffic", { params: { query: { range } } }),
      ),
    retry: false,
    enabled,
    meta: { silentError: true },
    refetchInterval: 60_000,
  });
}

export function useSaveRouterConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RouterConfigInput) =>
      withElevation(() => unwrap(routerApi.PUT("/router/config", { body }))),
    onSuccess: (data) => {
      qc.setQueryData(routerKeys.config, data);
      qc.invalidateQueries({ queryKey: routerKeys.status });
      qc.invalidateQueries({ queryKey: routerKeys.clients });
    },
  });
}

/** B114：生成上报令牌并切到“路由器主动上报”。令牌只在这次返回里出现。 */
export function useCreateRouterPushToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(routerApi.POST("/router/push/token"))),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: routerKeys.config });
      qc.invalidateQueries({ queryKey: routerKeys.status });
      qc.invalidateQueries({ queryKey: routerKeys.clients });
    },
  });
}

/** B114：路由器上报的间隔，脚本最晚下一分钟读到新值。 */
export function useSetPushInterval() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (seconds: 3 | 5 | 10 | 30 | 60) =>
      unwrap(routerApi.PUT("/router/push/interval", { body: { seconds } })),
    onSuccess: (data) => qc.setQueryData(routerKeys.config, data),
  });
}

export function useRestartInterface() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      withElevation(() =>
        unwrap(
          routerApi.POST("/router/interfaces/{name}/restart", {
            params: { path: { name } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: routerKeys.status }),
  });
}

export function useRebootRouter() {
  return useMutation({
    mutationFn: () =>
      withElevation(() =>
        unwrap(routerApi.POST("/router/reboot", { body: { confirm: "重启" } })),
      ),
  });
}

/** B228：面板记住的设备。跟着刷新频率，最快 3 秒，和最短的上报间隔一样。 */
export function useRouterDevices(enabled = true) {
  const chosen = useRouterInterval();
  return useQuery({
    queryKey: routerKeys.devices,
    queryFn: () => unwrap(routerApi.GET("/router/devices")),
    retry: false,
    enabled,
    meta: { silentError: true },
    refetchInterval: (q) =>
      chosen === 0 || isNotPush(q.state.error) ? false : Math.max(chosen, 3000),
  });
}

export interface DeviceSettings {
  note: string;
  notify: boolean;
  cooldownMinutes: number | null;
}

/** 改备注、通知开关和再次通知间隔。三项一起传。 */
export function useUpdateRouterDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ mac, body }: { mac: string; body: DeviceSettings }) =>
      unwrap(
        routerApi.PATCH("/router/devices/{mac}", {
          params: { path: { mac: macPath(mac) } },
          body,
        }),
      ),
    onSuccess: (device) => {
      qc.setQueryData(
        routerKeys.devices,
        (
          old: { items: RouterDevice[]; trafficAvailable: boolean } | undefined,
        ) =>
          old && {
            ...old,
            items: old.items.map((d) => (d.mac === device.mac ? device : d)),
          },
      );
      qc.invalidateQueries({ queryKey: routerKeys.clients });
    },
  });
}

/** 忘掉一台设备：删备注和流量，再出现时当新设备。 */
export function useForgetRouterDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mac: string) =>
      unwrap(
        routerApi.DELETE("/router/devices/{mac}", {
          params: { path: { mac: macPath(mac) } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: routerKeys.devices }),
  });
}

export function useRouterOptions() {
  return useQuery({
    queryKey: routerKeys.options,
    queryFn: () => unwrap(routerApi.GET("/router/options")),
    retry: false,
  });
}

export function useSaveRouterOptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RouterOptions) =>
      unwrap(routerApi.PUT("/router/options", { body })),
    onSuccess: (data) => qc.setQueryData(routerKeys.options, data),
  });
}
