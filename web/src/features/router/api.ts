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
export type RouterStatus = S["RouterStatus"];
export type RouterInterface = S["RouterInterface"];
export type RouterClient = S["RouterClient"];
export type RouterTraffic = S["RouterTraffic"];
export type TrafficRange = "24h" | "7d";

export const routerKeys = {
  all: ["router"] as const,
  config: ["router", "config"] as const,
  status: ["router", "status"] as const,
  clients: ["router", "clients"] as const,
  traffic: (range: TrafficRange) => ["router", "traffic", range] as const,
};

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
