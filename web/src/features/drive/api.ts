import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ApiError, createApi, errorMessage, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
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

// 演示数据（临时，见 src/demo）提供文件内容的本地地址。去掉演示数据时一起删。
const demoFileUrl = (id: number) =>
  (
    globalThis as { xcDemoFileUrl?: (id: number) => string | undefined }
  ).xcDemoFileUrl?.(id);

export function contentUrl(id: number, inline = false) {
  return (
    demoFileUrl(id) ??
    `/api/v1/drive/items/${id}/content${inline ? "?inline=1" : ""}`
  );
}

export function thumbnailUrl(item: DriveItem) {
  return (
    demoFileUrl(item.id) ??
    `/api/v1/drive/items/${item.id}/thumbnail?v=${encodeURIComponent(item.updatedAt)}`
  );
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
