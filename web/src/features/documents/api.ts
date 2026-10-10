import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/documents";
import { withElevation } from "../../auth/elevation";

/* 证件档案（B115）：护照、合同、保险、物品保修，到期前提醒。 */
export const documentsApi = createApi<paths>();

type S = components["schemas"];
export type DocumentItem = S["Document"];
export type DocumentInput = S["DocumentInput"];
export type DocumentPatch = S["DocumentPatch"];
export type DocumentKind = S["DocumentKind"];
export type DocumentStatus = S["DocumentStatus"];
export type DocumentFile = S["DocumentFile"];
export type DocumentSummary = S["DocumentSummary"];

export const documentKeys = {
  all: ["documents"] as const,
  list: (archived: boolean) => ["documents", "list", archived] as const,
};

// 服务端每次增删改发一次事件
invalidateOn("document.", documentKeys.all);

/** 全部档案。分类和搜索在页面里筛，数据量很小。 */
export function useDocuments(archived: boolean) {
  return useQuery({
    queryKey: documentKeys.list(archived),
    queryFn: () =>
      unwrap(
        documentsApi.GET("/documents", { params: { query: { archived } } }),
      ),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: documentKeys.all });
}

export function useCreateDocument() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: DocumentInput) =>
      unwrap(documentsApi.POST("/documents", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateDocument() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: DocumentPatch }) =>
      unwrap(
        documentsApi.PATCH("/documents/{documentId}", {
          params: { path: { documentId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteDocument() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          documentsApi.DELETE("/documents/{documentId}", {
            params: { path: { documentId: id } },
          }),
        ),
      ),
    onSuccess: refresh,
  });
}

export function useAddDocumentFile() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; file: DocumentFile }) =>
      unwrap(
        documentsApi.POST("/documents/{documentId}/files", {
          params: { path: { documentId: v.id } },
          body: v.file,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useRemoveDocumentFile() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; driveId: number }) =>
      unwrap(
        documentsApi.DELETE("/documents/{documentId}/files/{driveId}", {
          params: { path: { documentId: v.id, driveId: v.driveId } },
        }),
      ),
    onSuccess: refresh,
  });
}
