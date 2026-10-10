import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/readlater";

/* 稍后读（B117）：存链接，后台抓正文，AI 写摘要和标签。 */
export const readlaterApi = createApi<paths>();

type S = components["schemas"];
export type ReadItem = S["ReadItem"];
export type ReadList = S["ReadList"];
export type ReadItemPatch = S["ReadItemPatch"];
export type ReadItemResult = S["ReadItemResult"];

export type ReadView = "unread" | "read" | "all";

export const readKeys = {
  all: ["readlater"] as const,
  list: (view: ReadView, tag: string, q: string) =>
    ["readlater", "list", view, tag, q] as const,
  item: (id: number) => ["readlater", "item", id] as const,
};

// 服务端每次增删改、抓取完成都发一次事件
invalidateOn("readlater.", readKeys.all);

/** 还有没抓完的条目时，每 3 秒再取一次，事件丢了也能追上。 */
const POLL_MS = 3000;

export function useReadList(view: ReadView, tag: string, q: string) {
  return useQuery({
    queryKey: readKeys.list(view, tag, q),
    queryFn: () =>
      unwrap(
        readlaterApi.GET("/readlater", {
          params: {
            query: { view, ...(tag ? { tag } : {}), ...(q ? { q } : {}) },
          },
        }),
      ),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.items.some((i) => i.status === "queued")
        ? POLL_MS
        : false,
  });
}

export function useReadItem(id: number | null) {
  return useQuery({
    queryKey: readKeys.item(id ?? 0),
    enabled: id != null,
    queryFn: () =>
      unwrap(
        readlaterApi.GET("/readlater/{itemId}", {
          params: { path: { itemId: id ?? 0 } },
        }),
      ),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.status === "queued" ? POLL_MS : false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: readKeys.all });
}

export function useAddLink() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: {
      url: string;
      note?: string;
      source?: "web" | "share";
    }) => unwrap(readlaterApi.POST("/readlater", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateReadItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: ReadItemPatch }) =>
      unwrap(
        readlaterApi.PATCH("/readlater/{itemId}", {
          params: { path: { itemId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteReadItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        readlaterApi.DELETE("/readlater/{itemId}", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useRefetchReadItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        readlaterApi.POST("/readlater/{itemId}/refetch", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useSummarizeReadItem() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        readlaterApi.POST("/readlater/{itemId}/summarize", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}
