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
export type BackupTargetTest = S["BackupTargetTest"];
export type BackupRetention = S["BackupRetention"];
export type BackupSnapshot = S["BackupSnapshot"];
export type BackupRepoStats = S["BackupRepoStats"];
export type BackupSnapshotChange = S["BackupSnapshotChange"];

export const backupKeys = {
  all: ["backup"] as const,
  list: ["backup", "list"] as const,
  job: ["backup", "job"] as const,
  settings: ["backup", "settings"] as const,
  snapshots: ["backup", "snapshots"] as const,
  changes: (id: string) => ["backup", "snapshots", "changes", id] as const,
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
    // B81：立即备份要提升权限
    mutationFn: () =>
      withElevation(() => unwrap(backupApi.POST("/backups/run"))),
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

/** 用表单里的设置试一次备份位置，不保存（B63）。 */
export function useTestBackupTarget() {
  return useMutation({
    mutationFn: (body: BackupSettingsInput) =>
      withElevation(() =>
        unwrap(backupApi.POST("/backups/target/test", { body })),
      ),
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

/* ---------- B81：增量备份的快照 ---------- */

export function useSnapshots(enabled: boolean) {
  return useQuery({
    queryKey: backupKeys.snapshots,
    queryFn: () => unwrap(backupApi.GET("/backups/snapshots")),
    enabled,
    retry: false,
  });
}

/** 和上一个快照相比改了什么，展开时才查。 */
export function useSnapshotChanges(id: string | null) {
  return useQuery({
    queryKey: backupKeys.changes(id ?? ""),
    queryFn: () =>
      unwrap(
        backupApi.GET("/backups/snapshots/{snapshotId}/changes", {
          params: { path: { snapshotId: id! } },
        }),
      ),
    enabled: !!id,
    retry: false,
    staleTime: Infinity,
  });
}

export function useRestoreSnapshot() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: string) =>
      withElevation(() =>
        unwrap(
          backupApi.POST("/backups/snapshots/{snapshotId}/restore", {
            params: { path: { snapshotId: id } },
            body: { confirm: "恢复" },
          }),
        ),
      ),
    onSuccess: refresh,
  });
}

/** 检查每个快照引用的块都在。后台做，结果在 /backups/job 里。 */
export function useCheckBackup() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(backupApi.POST("/backups/check"))),
    onSuccess: refresh,
  });
}
