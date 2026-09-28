import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/backup";
import { withElevation } from "../../auth/elevation";

/* 整站导出、导入和自动备份（B25）。 */
export const backupApi = createApi<paths>();

type S = components["schemas"];
export type Backup = S["Backup"];
export type BackupJob = S["BackupJob"];
export type BackupSettings = S["BackupSettings"];
export type BackupSettingsInput = S["BackupSettingsInput"];

export const backupKeys = {
  all: ["backup"] as const,
  list: ["backup", "list"] as const,
  job: ["backup", "job"] as const,
  settings: ["backup", "settings"] as const,
};

invalidateOn("backup.", backupKeys.all);

export function useBackups() {
  return useQuery({
    queryKey: backupKeys.list,
    queryFn: () => unwrap(backupApi.GET("/backups")),
    retry: false,
  });
}

export function useBackupJob() {
  return useQuery({
    queryKey: backupKeys.job,
    queryFn: () => unwrap(backupApi.GET("/backups/job")),
    retry: false,
    refetchInterval: (q) => (q.state.data?.state === "running" ? 1500 : false),
  });
}

export function useBackupSettings() {
  return useQuery({
    queryKey: backupKeys.settings,
    queryFn: () => unwrap(backupApi.GET("/backups/settings")),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: backupKeys.all });
}

export function useExportBackup() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(backupApi.POST("/backups/export"))),
    onSuccess: refresh,
  });
}

export function useRunBackupNow() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () => unwrap(backupApi.POST("/backups/run")),
    onSuccess: refresh,
  });
}

export function useRestoreBackup() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: string) =>
      withElevation(() =>
        unwrap(
          backupApi.POST("/backups/{backupId}/restore", {
            params: { path: { backupId: id } },
            body: { confirm: "恢复" },
          }),
        ),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteBackup() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: string) =>
      withElevation(() =>
        unwrap(
          backupApi.DELETE("/backups/{backupId}", {
            params: { path: { backupId: id } },
          }),
        ),
      ),
    onSuccess: refresh,
  });
}

export function useSaveBackupSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BackupSettingsInput) =>
      withElevation(() => unwrap(backupApi.PUT("/backups/settings", { body }))),
    onSuccess: (data) => qc.setQueryData(backupKeys.settings, data),
  });
}

/** 上传备份包。文件可能很大，用 fetch 直接发 multipart。 */
export function useUploadBackup() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (file: File) =>
      withElevation(async () => {
        const form = new FormData();
        form.append("file", file);
        const res = await apiFetch("/backups/upload", {
          method: "POST",
          body: form,
        });
        return (await res.json()) as Backup;
      }),
    onSuccess: refresh,
  });
}

/** 下载地址。要登录和提升权限，浏览器直接打开这个地址。 */
export function downloadUrl(id: string) {
  return `/api/v1/backups/${encodeURIComponent(id)}/download`;
}
