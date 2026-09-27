import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { apiFetch, createApi, errorMessage, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/notes";
import { toast } from "../../hooks/useToast";

export const notesApi = createApi<paths>();

export type Note = components["schemas"]["Note"];
export type NoteSummary = components["schemas"]["NoteSummary"];
export type TagCount = components["schemas"]["TagCount"];
export type UpdateNote = components["schemas"]["UpdateNote"];

export interface NotesFilter {
  q: string;
  tag: string;
  archived: boolean;
}

export const notesKeys = {
  all: ["notes"] as const,
  lists: ["notes", "list"] as const,
  list: (f: NotesFilter) => ["notes", "list", f] as const,
  note: (id: number) => ["notes", "note", id] as const,
  tags: ["notes", "tags"] as const,
};

// 其他窗口改了笔记时刷新。编辑器自己决定要不要采用新内容（见 NoteEditor）。
invalidateOn("note.", notesKeys.all);

export function useNotes(filter: NotesFilter) {
  return useInfiniteQuery({
    queryKey: notesKeys.list(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        notesApi.GET("/notes", {
          params: {
            query: {
              q: filter.q || undefined,
              tag: filter.tag || undefined,
              archived: filter.archived || undefined,
              limit: 50,
              cursor: pageParam || undefined,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.nextCursor,
    placeholderData: (prev) => prev,
  });
}

export function useNote(id: number) {
  return useQuery({
    queryKey: notesKeys.note(id),
    queryFn: () =>
      unwrap(
        notesApi.GET("/notes/{noteId}", { params: { path: { noteId: id } } }),
      ),
  });
}

export function useTags() {
  return useQuery({
    queryKey: notesKeys.tags,
    queryFn: () => unwrap(notesApi.GET("/notes/tags")),
  });
}

export function patchNote(id: number, body: UpdateNote) {
  return unwrap(
    notesApi.PATCH("/notes/{noteId}", {
      params: { path: { noteId: id } },
      body,
    }),
  );
}

/** 页面关闭时用 keepalive 发出最后一次保存，浏览器会在后台把它发完。 */
export function patchNoteKeepalive(id: number, body: UpdateNote) {
  return apiFetch(`/notes/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    keepalive: true,
  }).catch(() => undefined);
}

const fail = (error: unknown) =>
  toast({ message: errorMessage(error), tone: "error" });

export function useCreateNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components["schemas"]["CreateNote"]) =>
      unwrap(notesApi.POST("/notes", { body })),
    onSuccess: (note) => {
      qc.setQueryData(notesKeys.note(note.id), note);
      qc.invalidateQueries({ queryKey: notesKeys.lists });
      qc.invalidateQueries({ queryKey: notesKeys.tags });
    },
    onError: fail,
  });
}

export function useUpdateNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateNote }) =>
      patchNote(id, body),
    onSuccess: (note) => {
      qc.setQueryData(notesKeys.note(note.id), note);
      qc.invalidateQueries({ queryKey: notesKeys.lists });
      qc.invalidateQueries({ queryKey: notesKeys.tags });
    },
    onError: fail,
  });
}

export function useDeleteNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        notesApi.DELETE("/notes/{noteId}", {
          params: { path: { noteId: id } },
        }),
      ),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: notesKeys.note(id) });
      qc.invalidateQueries({ queryKey: notesKeys.lists });
      qc.invalidateQueries({ queryKey: notesKeys.tags });
    },
    onError: fail,
  });
}

export function useNoteToIssue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, projectId }: { id: number; projectId: number }) =>
      unwrap(
        notesApi.POST("/notes/{noteId}/to-issue", {
          params: { path: { noteId: id } },
          body: { projectId },
        }),
      ),
    onSuccess: (out) => {
      qc.setQueryData(notesKeys.note(out.note.id), out.note);
      qc.invalidateQueries({ queryKey: notesKeys.lists });
      qc.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useNoteToReminder() {
  return useMutation({
    mutationFn: ({
      id,
      at,
      rrule,
    }: {
      id: number;
      at: string;
      rrule?: string;
    }) =>
      unwrap(
        notesApi.POST("/notes/{noteId}/to-reminder", {
          params: { path: { noteId: id } },
          body: { at, rrule },
        }),
      ),
  });
}
