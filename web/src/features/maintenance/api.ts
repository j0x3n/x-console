import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/maintenance";
import { withElevation } from "../../auth/elevation";
import { coreApi } from "../../api/core";

/* 设置 → 维护（B79）：版本、资源、存储占用、清理。 */
export const maintenanceApi = createApi<paths>();

type S = components["schemas"];
export type MaintenanceOverview = S["MaintenanceOverview"];
export type MaintenancePoint = S["MaintenancePoint"];
export type StorageUsage = S["StorageUsage"];
export type CleanupScan = S["CleanupScan"];
export type CleanupGroup = S["CleanupGroup"];

export const maintenanceKeys = {
  all: ["maintenance"] as const,
  overview: ["maintenance", "overview"] as const,
  metrics: ["maintenance", "metrics"] as const,
  scan: ["maintenance", "scan"] as const,
  cleanup: ["maintenance", "cleanup"] as const,
};

// 扫描和清理的进度变化时服务端发 maintenance.job，不用轮询。
invalidateOn("maintenance.job", maintenanceKeys.scan);
invalidateOn("maintenance.job", maintenanceKeys.cleanup);

export function useOverview() {
  return useQuery({
    queryKey: maintenanceKeys.overview,
    queryFn: () => unwrap(maintenanceApi.GET("/maintenance/overview")),
    retry: false,
  });
}

/** 进程资源每分钟采一次，这里也每分钟刷新。 */
export function useMetrics() {
  return useQuery({
    queryKey: maintenanceKeys.metrics,
    queryFn: () => unwrap(maintenanceApi.GET("/maintenance/metrics")),
    retry: false,
    refetchInterval: 60_000,
  });
}

export function useScan() {
  return useQuery({
    queryKey: maintenanceKeys.scan,
    queryFn: () => unwrap(maintenanceApi.GET("/maintenance/scan")),
    retry: false,
  });
}

export function useCleanupJob() {
  return useQuery({
    queryKey: maintenanceKeys.cleanup,
    queryFn: () => unwrap(maintenanceApi.GET("/maintenance/cleanup")),
    retry: false,
  });
}

/** 重新统计存储占用（服务端默认缓存 10 分钟）。 */
export function useRefreshOverview() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap(
        maintenanceApi.GET("/maintenance/overview", {
          params: { query: { refresh: true } },
        }),
      ),
    onSuccess: (data) => qc.setQueryData(maintenanceKeys.overview, data),
  });
}

export function useStartScan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(maintenanceApi.POST("/maintenance/scan")),
    onSuccess: (data) => qc.setQueryData(maintenanceKeys.scan, data),
  });
}

export function useStartCleanup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { kinds: string[]; scanId?: string }) =>
      withElevation(() =>
        unwrap(maintenanceApi.POST("/maintenance/cleanup", { body })),
      ),
    onSuccess: (data) => {
      qc.setQueryData(maintenanceKeys.cleanup, data);
      void qc.invalidateQueries({ queryKey: maintenanceKeys.overview });
    },
  });
}

export function useVacuum() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(maintenanceApi.POST("/maintenance/vacuum"))),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: maintenanceKeys.overview }),
  });
}

/** 前端构建时写进来的版本（vite.config.ts 的 define）。 */
export const webVersion = {
  version: __XC_VERSION__,
  builtAt: __XC_BUILT_AT__,
};

/** 服务端版本，/health 不用登录也能取。 */
export function useServerVersion() {
  return useQuery({
    queryKey: [...maintenanceKeys.all, "health"],
    queryFn: () => unwrap(coreApi.GET("/health")).then((r) => r.version),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

/**
 * 前后端版本对不上（浏览器缓存了旧页面）。
 * 开发时前端是 dev，不算对不上。
 */
export function versionMismatch(web: string, server?: string): boolean {
  return !!server && web !== "dev" && server !== "dev" && web !== server;
}

/** 已运行多久：3 天 4 小时、5 小时 12 分、8 分钟。 */
export function uptimeText(startedAt: string, now = Date.now()): string {
  const minutes = Math.max(
    0,
    Math.floor((now - Date.parse(startedAt)) / 60000),
  );
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  if (hours > 0) return `${hours} 小时 ${minutes % 60} 分`;
  return `${minutes} 分钟`;
}
