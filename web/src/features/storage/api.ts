import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/storage";
import { withElevation } from "../../auth/elevation";

/* 整站文件存储（B24）：本机磁盘或 S3，切换时后台搬文件。 */
export const storageApi = createApi<paths>();

type S = components["schemas"];
export type StorageStatus = S["StorageStatus"];
export type StorageBackend = S["StorageBackend"];
export type StorageS3 = S["StorageS3"];
export type StorageS3Input = S["StorageS3Input"];
export type TestResult = S["TestResult"];

export const storageKeys = {
  status: ["storage", "status"] as const,
};

invalidateOn("storage.", storageKeys.status);

export function useStorage() {
  return useQuery({
    queryKey: storageKeys.status,
    queryFn: () => unwrap(storageApi.GET("/storage")),
    retry: false,
    // 正在搬文件时每 2 秒看一次进度。
    refetchInterval: (q) =>
      q.state.data?.migration.state === "running" ? 2000 : false,
  });
}

export function useSaveStorageS3() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: StorageS3Input) =>
      withElevation(() => unwrap(storageApi.PUT("/storage/s3", { body }))),
    onSuccess: (data) => qc.setQueryData(storageKeys.status, data),
  });
}

export function useTestStorageS3() {
  return useMutation({
    mutationFn: (body: StorageS3Input) =>
      unwrap(storageApi.POST("/storage/s3/test", { body })),
  });
}

export function useSwitchStorage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { target: StorageBackend; deleteSource: boolean }) =>
      withElevation(() => unwrap(storageApi.POST("/storage/switch", { body }))),
    onSuccess: () => qc.invalidateQueries({ queryKey: storageKeys.status }),
  });
}

export function useCancelSwitch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(storageApi.POST("/storage/switch/cancel")),
    onSuccess: () => qc.invalidateQueries({ queryKey: storageKeys.status }),
  });
}

export function useSaveCache() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (limitBytes: number) =>
      unwrap(storageApi.PUT("/storage/cache", { body: { limitBytes } })),
    onSuccess: (data) => qc.setQueryData(storageKeys.status, data),
  });
}

/** files.Store 里第一段 key 对应的中文名。 */
export const moduleLabels: Record<string, string> = {
  drive: "Drive",
  "drive-versions": "File history",
  notes: "Note attachments",
  projects: "Project images",
  ai: "AI attachments",
};
