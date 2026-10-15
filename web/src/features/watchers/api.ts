import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { API_BASE, ApiError, createApi, unwrap } from "../../api/client";
import { coreApi } from "../../api/core";
import type { components, paths } from "../../api/gen/watchers";
import { withElevation } from "../../auth/elevation";

/* 网站自动化（B206 网页盯梢，B213 改名）：AI 摸索一次记下步骤，平时回放，变了通知。 */
export const watchersApi = createApi<paths>();

type S = components["schemas"];
export type Watcher = S["Watcher"];
export type WatcherDetail = S["WatcherDetail"];
export type WatcherInput = S["WatcherInput"];
export type WatcherPatch = S["WatcherPatch"];
export type WatcherStatus = S["WatcherStatus"];
export type WatcherKind = S["WatcherKind"];
export type WatcherRun = S["WatcherRun"];
export type WatchersStatus = S["WatchersStatus"];
export type WatchersSettings = S["WatchersSettings"];

export const watcherKeys = {
  all: ["watchers"] as const,
  list: ["watchers", "list"] as const,
  one: (id: number) => ["watchers", "one", id] as const,
  status: ["watchers", "status"] as const,
  settings: ["watchers", "settings"] as const,
};

// 服务端每次状态变化发 watcher.updated
invalidateOn("watcher.", watcherKeys.all);

export function useWatchers() {
  return useQuery({
    queryKey: watcherKeys.list,
    queryFn: async () => (await unwrap(watchersApi.GET("/watchers"))).items,
    retry: false,
  });
}

export function useWatcher(id: number) {
  return useQuery({
    queryKey: watcherKeys.one(id),
    queryFn: () =>
      unwrap(
        watchersApi.GET("/watchers/{watcherId}", {
          params: { path: { watcherId: id } },
        }),
      ),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useWatchersStatus() {
  return useQuery({
    queryKey: watcherKeys.status,
    queryFn: () => unwrap(watchersApi.GET("/watchers/status")),
    retry: false,
    meta: { silentError: true },
  });
}

export function useWatchersSettings(enabled = true) {
  return useQuery({
    queryKey: watcherKeys.settings,
    queryFn: () => unwrap(watchersApi.GET("/watchers/settings")),
    enabled,
  });
}

export function usePutWatchersSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: WatchersSettings) =>
      unwrap(watchersApi.PUT("/watchers/settings", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

export function useCreateWatcher() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: WatcherInput) =>
      withElevation(() => unwrap(watchersApi.POST("/watchers", { body }))),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

export function useUpdateWatcher() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: number; body: WatcherPatch }) =>
      withElevation(() =>
        unwrap(
          watchersApi.PATCH("/watchers/{watcherId}", {
            params: { path: { watcherId: v.id } },
            body: v.body,
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

export function useDeleteWatcher() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        watchersApi.DELETE("/watchers/{watcherId}", {
          params: { path: { watcherId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

export type RunKind = "check" | "explore" | "manual";

export function useRunWatcher() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: number; kind: RunKind }) =>
      unwrap(
        watchersApi.POST("/watchers/{watcherId}/run", {
          params: { path: { watcherId: v.id } },
          body: { kind: v.kind },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

export type Answer = "done" | "approve" | "deny" | "cancel";

export function useAnswerWatcher() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: number; answer: Answer }) =>
      unwrap(
        watchersApi.POST("/watchers/{watcherId}/answer", {
          params: { path: { watcherId: v.id } },
          body: { answer: v.answer },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: watcherKeys.all }),
  });
}

/** 截图地址。 */
export function shotUrl(id: number) {
  return `${API_BASE}/watchers/shots/${id}`;
}

/** 接管页面要先验证过，WebSocket 自己弹不出验证码框。 */
export async function ensureElevated(): Promise<void> {
  const status = await unwrap(coreApi.GET("/auth/status"));
  const until = status.elevatedUntil
    ? new Date(status.elevatedUntil).getTime()
    : 0;
  if (until - Date.now() < 10_000) {
    throw new ApiError(403, "elevation_required", "需要再次验证");
  }
}
