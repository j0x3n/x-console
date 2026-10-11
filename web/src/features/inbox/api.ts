import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/inbox";

/* 随手记收集箱（B134）。 */
export const inboxApi = createApi<paths>();

type S = components["schemas"];
export type InboxItem = S["InboxItem"];
export type InboxKind = S["InboxKind"];
export type InboxSuggestion = S["InboxSuggestion"];
export type InboxFileInput = S["InboxFileInput"];
export type InboxSettings = S["InboxSettings"];

export const inboxKeys = {
  all: ["inbox"] as const,
  open: ["inbox", "open"] as const,
  closed: ["inbox", "closed"] as const,
  settings: ["inbox", "settings"] as const,
};

// 服务端每次收、归档、忽略、删除、生成完建议都发 inbox.changed
invalidateOn("inbox.", inboxKeys.all);

export function useInbox() {
  return useQuery({
    queryKey: inboxKeys.open,
    queryFn: () =>
      unwrap(inboxApi.GET("/inbox", { params: { query: { status: "open" } } })),
    retry: false,
  });
}

export function useClosedInbox(enabled: boolean) {
  return useQuery({
    queryKey: inboxKeys.closed,
    queryFn: () =>
      unwrap(
        inboxApi.GET("/inbox", { params: { query: { status: "closed" } } }),
      ),
    enabled,
    retry: false,
  });
}

export function useInboxSettings() {
  return useQuery({
    queryKey: inboxKeys.settings,
    queryFn: () => unwrap(inboxApi.GET("/inbox/settings")),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: inboxKeys.all });
}

export function useAddInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (text: string) =>
      unwrap(inboxApi.POST("/inbox", { body: { text } })),
    onSuccess: refresh,
  });
}

/** 上传一段录音，服务端转成文字后收进来。 */
export function useAddVoice() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async (v: { blob: Blob; language?: string }) => {
      const query = v.language
        ? `?language=${encodeURIComponent(v.language)}`
        : "";
      const res = await apiFetch(`/inbox/voice${query}`, {
        method: "POST",
        headers: { "Content-Type": v.blob.type || "application/octet-stream" },
        body: v.blob,
      });
      return (await res.json()) as InboxItem;
    },
    onSuccess: refresh,
  });
}

export function useEditInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; text: string }) =>
      unwrap(
        inboxApi.PATCH("/inbox/{itemId}", {
          params: { path: { itemId: v.id } },
          body: { text: v.text },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useSuggestInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        inboxApi.POST("/inbox/{itemId}/suggest", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useFileInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: InboxFileInput }) =>
      unwrap(
        inboxApi.POST("/inbox/{itemId}/file", {
          params: { path: { itemId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDismissInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        inboxApi.POST("/inbox/{itemId}/dismiss", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteInbox() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        inboxApi.DELETE("/inbox/{itemId}", {
          params: { path: { itemId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function usePutInboxSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: InboxSettings) =>
      unwrap(inboxApi.PUT("/inbox/settings", { body })),
    onSuccess: refresh,
  });
}
