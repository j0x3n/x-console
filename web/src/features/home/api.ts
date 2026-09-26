import { useCallback } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import { invalidateOn, useServerEvent, type ServerEvent } from "../../api/events";
import { withElevation } from "../../auth/elevation";
import type { components, paths } from "../../api/gen/homeassistant";
import { applyStateChange } from "./logic";

export const haApi = createApi<paths>();

export type HAState = components["schemas"]["HAState"];
export type HAStatus = components["schemas"]["HAStatus"];
export type HAConfig = components["schemas"]["HAConfig"];
export type HAConfigInput = components["schemas"]["HAConfigInput"];
export type HAFavorite = components["schemas"]["HAFavorite"];
export type HAFavoriteInput = components["schemas"]["HAFavoriteInput"];
export type HAMode = components["schemas"]["HAMode"];
export type HATestResult = components["schemas"]["HATestResult"];

export const haKeys = {
  all: ["ha"] as const,
  status: ["ha", "status"] as const,
  config: ["ha", "config"] as const,
  states: ["ha", "states"] as const,
  favorites: ["ha", "favorites"] as const,
};

// 连上或断开时整体刷新。状态变化由 useHAEvents 直接写进缓存。
invalidateOn("ha.connection_changed", haKeys.all);

export function isNotConfigured(error: unknown) {
  return (
    error instanceof ApiError && error.code === "integration_not_configured"
  );
}

export function useHAStatus() {
  return useQuery({
    queryKey: haKeys.status,
    queryFn: () => unwrap(haApi.GET("/ha/status")),
  });
}

export function useHAConfig() {
  return useQuery({
    queryKey: haKeys.config,
    queryFn: () => unwrap(haApi.GET("/ha/config")),
  });
}

/** 全部实体。过滤在前端做，数据量一般只有几百条。 */
export function useHAStates(enabled = true) {
  return useQuery({
    queryKey: haKeys.states,
    queryFn: () => unwrap(haApi.GET("/ha/states")),
    enabled,
  });
}

export function useHAFavorites() {
  return useQuery({
    queryKey: haKeys.favorites,
    queryFn: () => unwrap(haApi.GET("/ha/favorites")),
  });
}

/** 把实体状态写进缓存里的列表和收藏。 */
export function writeState(qc: QueryClient, state: HAState) {
  qc.setQueryData<HAState[]>(haKeys.states, (list) =>
    list ? applyStateChange(list, state) : list,
  );
  qc.setQueryData<HAFavorite[]>(haKeys.favorites, (favs) =>
    favs?.map((f) => (f.entityId === state.entityId ? { ...f, state } : f)),
  );
}

/** 页面打开时接收 ha.state_changed，直接更新缓存，不重新请求。 */
export function useHAEvents() {
  const qc = useQueryClient();
  const onEvent = useCallback(
    (event: ServerEvent) => {
      if (event.topic === "ha.state_changed")
        writeState(qc, event.data as HAState);
    },
    [qc],
  );
  useServerEvent("ha.", onEvent);
}

export interface ServiceCall {
  domain: string;
  service: string;
  entityId?: string;
  data?: Record<string, unknown>;
}

/** 调 HA 服务。门锁这类操作服务端要求再次验证，这里会弹出验证码框。 */
export function useCallService() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ domain, service, entityId, data }: ServiceCall) =>
      withElevation(() =>
        unwrap(
          haApi.POST("/ha/services/{domain}/{service}", {
            params: { path: { domain, service } },
            body: { entityId, data },
          }),
        ),
      ),
    onSuccess: (_data, call) => {
      // 不是收藏的实体没有事件推送，调用后刷新一次列表。
      if (call.entityId) {
        const favs = qc.getQueryData<HAFavorite[]>(haKeys.favorites);
        if (!favs?.some((f) => f.entityId === call.entityId))
          window.setTimeout(
            () => qc.invalidateQueries({ queryKey: haKeys.states }),
            600,
          );
      }
    },
  });
}

export function useSaveFavorites() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (items: HAFavoriteInput[]) =>
      unwrap(haApi.PUT("/ha/favorites", { body: { items } })),
    onSuccess: (data) => qc.setQueryData(haKeys.favorites, data),
  });
}

export function useSaveConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: HAConfigInput) =>
      withElevation(() => unwrap(haApi.PUT("/ha/config", { body }))),
    onSuccess: (data) => {
      qc.setQueryData(haKeys.config, data);
      qc.invalidateQueries({ queryKey: haKeys.all });
    },
  });
}

export function useTestHA() {
  return useMutation({
    mutationFn: (body: HAConfigInput) =>
      withElevation(() => unwrap(haApi.POST("/ha/test", { body }))),
  });
}
