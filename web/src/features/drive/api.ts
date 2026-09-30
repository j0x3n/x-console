import { useEffect } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ApiError,
  apiFetch,
  createApi,
  errorMessage,
  unwrap,
} from "../../api/client";
import { invalidateOn, onServerEvent } from "../../api/events";
import type { components, paths } from "../../api/gen/drive";
import { withElevation } from "../../auth/elevation";
import { toast } from "../../hooks/useToast";

export const driveApi = createApi<paths>();

export type DriveItem = components["schemas"]["DriveItem"];
export type DriveItemWithPath = components["schemas"]["DriveItemWithPath"];
export type UpdateDriveItem = components["schemas"]["UpdateDriveItem"];
export type S3Config = components["schemas"]["S3Config"];
export type S3ConfigInput = components["schemas"]["S3ConfigInput"];
export type S3Status = components["schemas"]["S3Status"];

/** 列表的范围：某个目录、全盘搜索、回收站、隐藏。 */
export interface DriveScope {
  folder: number | null;
  q: string;
  trash: boolean;
  hidden: boolean;
}

export const driveKeys = {
  all: ["drive"] as const,
  lists: ["drive", "list"] as const,
  list: (s: DriveScope) => ["drive", "list", s] as const,
  item: (id: number) => ["drive", "item", id] as const,
  folders: (parent: number | null) => ["drive", "folders", parent] as const,
  usage: ["drive", "usage"] as const,
  s3: ["drive", "s3"] as const,
  s3Status: ["drive", "s3", "status"] as const,
};

invalidateOn("drive_item.", driveKeys.all);
invalidateOn("drive.sync_status", driveKeys.s3Status);

/**
 * 服务端还没有云盘接口（404 或 501）。只用在 /drive/usage 上判断：
 * 这个接口上线后不会回 404，别的接口的 404 可能是条目不存在。
 */
export function isNotLive(error: unknown) {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  );
}

export function contentUrl(id: number, inline = false) {
  return `/api/v1/drive/items/${id}/content${inline ? "?inline=1" : ""}`;
}

export function thumbnailUrl(item: DriveItem) {
  return `/api/v1/drive/items/${item.id}/thumbnail?v=${encodeURIComponent(item.updatedAt)}`;
}

export function useDriveItems(scope: DriveScope) {
  return useQuery({
    queryKey: driveKeys.list(scope),
    queryFn: () =>
      unwrap(
        driveApi.GET("/drive/items", {
          params: {
            query: {
              parent: scope.q ? undefined : (scope.folder ?? undefined),
              q: scope.q || undefined,
              trashed: scope.trash || undefined,
              hidden: scope.hidden || undefined,
            },
          },
        }),
      ).then((r) => r.items),
    placeholderData: keepPreviousData,
  });
}

/** 只列文件夹，移动对话框用。 */
export function useDriveFolders(parent: number | null, hidden: boolean) {
  return useQuery({
    queryKey: [...driveKeys.folders(parent), hidden],
    queryFn: () =>
      unwrap(
        driveApi.GET("/drive/items", {
          params: {
            query: {
              parent: parent ?? undefined,
              hidden: hidden || undefined,
            },
          },
        }),
      ).then((r) => r.items.filter((i) => i.isDir)),
  });
}

export function useDriveItem(id: number | null) {
  return useQuery({
    queryKey: driveKeys.item(id ?? 0),
    queryFn: () =>
      unwrap(
        driveApi.GET("/drive/items/{itemId}", {
          params: { path: { itemId: id! } },
        }),
      ),
    enabled: id != null,
  });
}

export function useDriveUsage() {
  return useQuery({
    queryKey: driveKeys.usage,
    queryFn: () => unwrap(driveApi.GET("/drive/usage")),
  });
}

export function useS3Status() {
  return useQuery({
    queryKey: driveKeys.s3Status,
    queryFn: () => unwrap(driveApi.GET("/drive/s3/status")),
  });
}

export function useS3Config() {
  return useQuery({
    queryKey: driveKeys.s3,
    queryFn: () => unwrap(driveApi.GET("/drive/s3")),
  });
}

const fail = (error: unknown) =>
  toast({ message: errorMessage(error), tone: "error" });

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: driveKeys.all });
}

export function useCreateFolder() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: { parentId?: number; name: string; hidden?: boolean }) =>
      unwrap(driveApi.POST("/drive/folders", { body })),
    onSuccess: refresh,
    onError: fail,
  });
}

export function useUpdateItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateDriveItem }) =>
      unwrap(
        driveApi.PATCH("/drive/items/{itemId}", {
          params: { path: { itemId: id } },
          body,
        }),
      ),
    onSuccess: refresh,
    onError: fail,
  });
}

/** 批量操作：一个一个发，返回失败的数量。 */
async function each(ids: number[], fn: (id: number) => Promise<unknown>) {
  let failed = 0;
  for (const id of ids) {
    try {
      await fn(id);
    } catch (error) {
      failed++;
      fail(error);
    }
  }
  return failed;
}

export function useMoveItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ ids, parentId }: { ids: number[]; parentId: number }) =>
      each(ids, (id) =>
        unwrap(
          driveApi.PATCH("/drive/items/{itemId}", {
            params: { path: { itemId: id } },
            body: { parentId },
          }),
        ),
      ),
    onSettled: refresh,
  });
}

export function useTrashItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (ids: number[]) =>
      each(ids, (id) =>
        unwrap(
          driveApi.DELETE("/drive/items/{itemId}", {
            params: { path: { itemId: id } },
          }),
        ),
      ),
    onSettled: refresh,
  });
}

export function useDeleteForever() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (ids: number[]) =>
      withElevation(() =>
        each(ids, (id) =>
          unwrap(
            driveApi.DELETE("/drive/items/{itemId}", {
              params: { path: { itemId: id }, query: { permanent: true } },
            }),
          ),
        ),
      ),
    onSettled: refresh,
  });
}

export function useRestoreItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (ids: number[]) =>
      each(ids, (id) =>
        unwrap(
          driveApi.POST("/drive/items/{itemId}/restore", {
            params: { path: { itemId: id } },
          }),
        ),
      ),
    onSettled: refresh,
  });
}

export function useSaveS3Config() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: S3ConfigInput) =>
      withElevation(() => unwrap(driveApi.PUT("/drive/s3", { body }))),
    onSuccess: (cfg) => {
      qc.setQueryData(driveKeys.s3, cfg);
      qc.invalidateQueries({ queryKey: driveKeys.s3Status });
    },
  });
}

export function useTestS3() {
  return useMutation({
    mutationFn: (body: S3ConfigInput | null) =>
      unwrap(driveApi.POST("/drive/s3/test", body ? { body } : {})),
  });
}

export function useSyncS3() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(driveApi.POST("/drive/s3/sync")),
    onSuccess: () => qc.invalidateQueries({ queryKey: driveKeys.s3Status }),
    onError: fail,
  });
}

/** 读文件内容。range 是 HTTP Range 的值，比如 "bytes=-1048576"。 */
export async function readContent(
  id: number,
  range?: string,
): Promise<{ bytes: Uint8Array; etag: string }> {
  const res = await apiFetch(`/drive/items/${id}/content?inline=1`, {
    headers: range ? { Range: range } : undefined,
    cache: "no-store",
  });
  const bytes = new Uint8Array(await res.arrayBuffer());
  return { bytes, etag: res.headers.get("ETag") ?? "" };
}

/**
 * 保存文本。带上打开时的 etag，文件在别处改过时抛出 409 的 ApiError。
 * etag 传空就是直接覆盖。
 */
export async function saveContent(
  id: number,
  text: string,
  etag: string,
): Promise<{ item: DriveItem; etag: string }> {
  const headers: Record<string, string> = {
    "Content-Type": "text/plain; charset=utf-8",
  };
  if (etag) headers["If-Match"] = etag;
  const res = await apiFetch(`/drive/items/${id}/content`, {
    method: "PUT",
    headers,
    body: text,
  });
  return {
    item: (await res.json()) as DriveItem,
    etag: res.headers.get("ETag") ?? "",
  };
}

/* ---------- B31：批量操作、压缩解压、后台任务 ---------- */

export type DriveTask = components["schemas"]["DriveTask"];
export type ConflictPolicy = components["schemas"]["ConflictPolicy"];
export type DriveVersion = components["schemas"]["DriveVersion"];
export type DriveVersionSettings =
  components["schemas"]["DriveVersionSettings"];
export type DriveShare = components["schemas"]["DriveShare"];
export type DriveShareInput = components["schemas"]["DriveShareInput"];
export type DriveFollowFrame = components["schemas"]["DriveFollowFrame"];

export const taskKeys = {
  all: ["drive", "tasks"] as const,
  versions: (id: number) => ["drive", "versions", id] as const,
  versionSettings: ["drive", "version-settings"] as const,
  shares: (itemId?: number) => ["drive", "shares", itemId ?? 0] as const,
  allShares: ["drive", "shares"] as const,
};

invalidateOn("drive_share.", taskKeys.allShares);

/** 多个文件打包下载的地址。 */
export function zipUrl(ids: number[]) {
  const q = new URLSearchParams();
  ids.forEach((id) => q.append("ids", String(id)));
  return `/api/v1/drive/zip?${q}`;
}

function isRunning(task: DriveTask) {
  return task.state === "running";
}

/**
 * 后台任务列表。有任务在跑时每秒刷新一次，事件到得慢也不会卡住。
 * 这个接口回 404 或 501 时，说明服务端还没有批量操作（B31 后端没上线）。
 */
export function useDriveTasks() {
  return useQuery({
    queryKey: taskKeys.all,
    queryFn: () => unwrap(driveApi.GET("/drive/tasks")).then((r) => r.items),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some(isRunning) ? 1000 : false,
  });
}

/** 服务端有没有 B31 的批量接口。还不知道时当作有。 */
export function useBatchLive() {
  const tasks = useDriveTasks();
  return !(tasks.isError && isNotLive(tasks.error));
}

/** 事件推来的任务状态写进缓存；任务结束时刷新文件列表。 */
export function useTaskEvents() {
  const qc = useQueryClient();
  useEffect(
    () =>
      onServerEvent((event) => {
        if (event.topic !== "drive_task.updated") return;
        const task = event.data as DriveTask;
        qc.setQueryData<DriveTask[]>(taskKeys.all, (cur) => upsert(cur, task));
        if (!isRunning(task))
          void qc.invalidateQueries({ queryKey: driveKeys.all });
      }),
    [qc],
  );
}

export function upsert(list: DriveTask[] | undefined, task: DriveTask) {
  const cur = list ?? [];
  const i = cur.findIndex((x) => x.id === task.id);
  if (i < 0) return [task, ...cur];
  const next = cur.slice();
  next[i] = task;
  return next;
}

/** 服务端还没上线时的提示。 */
export function notLiveToast() {
  toast({ message: "服务端还没上线这个功能。", tone: "error" });
}

/** 开一个后台任务：成功后放进任务列表，后面由任务面板显示进度。 */
function useStartTask<T>(start: (body: T) => Promise<DriveTask>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: start,
    onSuccess: (task) => {
      qc.setQueryData<DriveTask[]>(taskKeys.all, (cur) => upsert(cur, task));
      if (isRunning(task)) return;
      // 很快的操作（比如同一个盘里移动）返回时已经做完了，任务面板不会显示，这里直接提示。
      void qc.invalidateQueries({ queryKey: driveKeys.all });
      if (task.state === "failed")
        toast({ message: `${task.title}：${task.error ?? ""}`, tone: "error" });
      else toast(`${task.title}：已完成`);
    },
    onError: (error) => (isNotLive(error) ? notLiveToast() : fail(error)),
  });
}

export interface TransferBody {
  ids: number[];
  targetId: number;
  conflict: ConflictPolicy;
}

export function useCopyItems() {
  return useStartTask((body: TransferBody) =>
    unwrap(driveApi.POST("/drive/batch/copy", { body })),
  );
}

export function useBatchMove() {
  return useStartTask((body: TransferBody) =>
    unwrap(driveApi.POST("/drive/batch/move", { body })),
  );
}

export function useArchive() {
  return useStartTask((body: components["schemas"]["ArchiveRequest"]) =>
    unwrap(driveApi.POST("/drive/archive", { body })),
  );
}

export function useExtract() {
  return useStartTask(
    ({
      id,
      ...body
    }: { id: number } & components["schemas"]["ExtractRequest"]) =>
      unwrap(
        driveApi.POST("/drive/items/{itemId}/extract", {
          params: { path: { itemId: id } },
          body,
        }),
      ),
  );
}

export function useCancelTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(
        driveApi.POST("/drive/tasks/{taskId}/cancel", {
          params: { path: { taskId: id } },
        }),
      ),
    onSuccess: (task) =>
      qc.setQueryData<DriveTask[]>(taskKeys.all, (cur) => upsert(cur, task)),
    onError: fail,
  });
}

/** 目标文件夹里已有的名字，用来在复制、移动前判断有没有重名。 */
export async function namesIn(parent: number, hidden: boolean) {
  const r = await unwrap(
    driveApi.GET("/drive/items", {
      params: {
        query: {
          parent: parent || undefined,
          hidden: hidden || undefined,
        },
      },
    }),
  );
  return new Set(r.items.map((i) => i.name));
}

/* ---------- B31：历史版本 ---------- */

export function useDriveVersions(id: number) {
  return useQuery({
    queryKey: taskKeys.versions(id),
    queryFn: () =>
      unwrap(
        driveApi.GET("/drive/items/{itemId}/versions", {
          params: { path: { itemId: id } },
        }),
      ).then((r) => r.items),
    retry: false,
  });
}

/** 读某个历史版本的文本。 */
export async function readVersion(id: number, versionId: number) {
  const res = await apiFetch(
    `/drive/items/${id}/versions/${versionId}/content`,
    { cache: "no-store" },
  );
  return new Uint8Array(await res.arrayBuffer());
}

export function useRestoreVersion(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (versionId: number) =>
      unwrap(
        driveApi.POST("/drive/items/{itemId}/versions/{versionId}/restore", {
          params: { path: { itemId: id, versionId } },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: taskKeys.versions(id) });
      void qc.invalidateQueries({ queryKey: driveKeys.all });
    },
    onError: fail,
  });
}

export function useVersionSettings() {
  return useQuery({
    queryKey: taskKeys.versionSettings,
    queryFn: () => unwrap(driveApi.GET("/drive/version-settings")),
    retry: false,
  });
}

export function useSaveVersionSettings() {
  const qc = useQueryClient();
  return useMutation({
    // 改小保留数量或天数会马上删掉旧版本，服务端要求提升权限。
    mutationFn: (body: DriveVersionSettings) =>
      withElevation(() =>
        unwrap(driveApi.PUT("/drive/version-settings", { body })),
      ),
    onSuccess: (data) => qc.setQueryData(taskKeys.versionSettings, data),
    onError: fail,
  });
}

/* ---------- B31：外链分享 ---------- */

export function useDriveShares(itemId?: number) {
  return useQuery({
    queryKey: taskKeys.shares(itemId),
    queryFn: () =>
      unwrap(
        driveApi.GET("/drive/shares", {
          params: { query: { itemId } },
        }),
      ).then((r) => r.items),
    retry: false,
  });
}

export function useCreateShare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DriveShareInput) =>
      unwrap(driveApi.POST("/drive/shares", { body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: taskKeys.allShares });
      void qc.invalidateQueries({ queryKey: driveKeys.lists });
    },
    onError: (error) => (isNotLive(error) ? notLiveToast() : fail(error)),
  });
}

export function useDeleteShare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        driveApi.DELETE("/drive/shares/{shareId}", {
          params: { path: { shareId: id } },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: taskKeys.allShares });
      void qc.invalidateQueries({ queryKey: driveKeys.lists });
    },
    onError: fail,
  });
}
