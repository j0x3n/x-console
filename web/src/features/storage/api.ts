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

// ---- B69 网盘账号：WebDAV、Google Drive，备份和云盘页共用 ----

export type StorageRemote = S["StorageRemote"];
export type StorageRemoteInput = S["StorageRemoteInput"];
export type StorageRemoteKind = S["StorageRemoteKind"];
export type StorageRemoteEntry = S["StorageRemoteEntry"];

export const remoteKeys = {
  all: ["storage", "remotes"] as const,
  list: ["storage", "remotes", "list"] as const,
  drive: ["storage", "remotes", "drive"] as const,
  items: (id: number, ref: string) =>
    ["storage", "remotes", "items", id, ref] as const,
};

invalidateOn("storage.remotes", remoteKeys.all);

/** 全部网盘账号，设置页和备份设置用。 */
export function useRemotes() {
  return useQuery({
    queryKey: remoteKeys.list,
    queryFn: () => unwrap(storageApi.GET("/storage/remotes")),
    retry: false,
  });
}

/** 云盘页要显示的账号。接口没上线或出错时当作没有，云盘页照常。 */
export function useDriveRemotes() {
  return useQuery({
    queryKey: remoteKeys.drive,
    queryFn: () =>
      unwrap(
        storageApi.GET("/storage/remotes", {
          params: { query: { drive: true } },
        }),
      ),
    retry: false,
    staleTime: 60_000,
    meta: { silentError: true },
  });
}

/** 添加（没有 id）或修改账号。 */
export function useSaveRemote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id?: number; body: StorageRemoteInput }) =>
      withElevation(() =>
        unwrap(
          id
            ? storageApi.PATCH("/storage/remotes/{remoteId}", {
                params: { path: { remoteId: id } },
                body,
              })
            : storageApi.POST("/storage/remotes", { body }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: remoteKeys.all }),
  });
}

export function useDeleteRemote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          storageApi.DELETE("/storage/remotes/{remoteId}", {
            params: { path: { remoteId: id } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: remoteKeys.all }),
  });
}

/** 用表单里的设置连一次，不保存。传 id 时没填的密码用已保存的。 */
export function useTestRemote() {
  return useMutation({
    mutationFn: ({ id, body }: { id?: number; body: StorageRemoteInput }) =>
      withElevation(() =>
        unwrap(
          storageApi.POST("/storage/remotes/test", {
            params: { query: id ? { id } : {} },
            body,
          }),
        ),
      ),
  });
}

export function useStartRemoteAuth() {
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          storageApi.GET("/storage/remotes/{remoteId}/gdrive/auth", {
            params: { path: { remoteId: id } },
          }),
        ),
      ),
  });
}

export function useRevokeRemoteAuth() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          storageApi.DELETE("/storage/remotes/{remoteId}/gdrive/auth", {
            params: { path: { remoteId: id } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: remoteKeys.all }),
  });
}

export function useRemoteItems(id: number, ref: string) {
  return useQuery({
    queryKey: remoteKeys.items(id, ref),
    queryFn: () =>
      unwrap(
        storageApi.GET("/storage/remotes/{remoteId}/items", {
          params: { path: { remoteId: id }, query: ref ? { ref } : {} },
        }),
      ),
    retry: false,
  });
}

export function remoteDownloadUrl(id: number, ref: string) {
  return `/api/v1/storage/remotes/${id}/download?ref=${encodeURIComponent(ref)}`;
}

/** 新加的 Google Drive 账号要填到 Google 控制台的重定向地址。 */
export function gdriveRedirectUri() {
  return `${window.location.origin}/api/v1/storage/remotes/gdrive/callback`;
}
