import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ApiError,
  apiFetch,
  createApi,
  errorMessage,
  isNotLive,
  unwrap,
} from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/notes";
import { toast } from "../../hooks/useToast";
import { queryClient } from "../../api/query";
import { withElevation } from "../../auth/elevation";

export const notesApi = createApi<paths>();

export type Note = components["schemas"]["Note"];
export type NoteSummary = components["schemas"]["NoteSummary"];
export type TagCount = components["schemas"]["TagCount"];
export type UpdateNote = components["schemas"]["UpdateNote"];
export type Attachment = components["schemas"]["Attachment"];
export type NoteAiSettings = components["schemas"]["NoteAiSettings"];
export type NoteAiSettingsInput = components["schemas"]["NoteAiSettingsInput"];
export type NoteKind = components["schemas"]["NoteKind"];
export type NoteColor = components["schemas"]["NoteColor"];
export type NoteShare = components["schemas"]["NoteShare"];
export type NoteShareInput = components["schemas"]["NoteShareInput"];
export type NoteCounts = components["schemas"]["NoteCounts"];

export interface NotesFilter {
  q: string;
  tag: string;
  archived: boolean;
  pinned: boolean;
  /** B13：只看隐藏笔记，要先解锁 */
  hidden: boolean;
  /** B73：只看笔记或只看便签，不传时两种都要 */
  kind?: NoteKind;
}

export const notesKeys = {
  all: ["notes"] as const,
  lists: ["notes", "list"] as const,
  list: (f: NotesFilter) => ["notes", "list", f] as const,
  note: (id: number) => ["notes", "note", id] as const,
  tags: ["notes", "tags"] as const,
  aiSettings: ["notes", "ai-settings"] as const,
  counts: ["notes", "counts"] as const,
  share: (id: number) => ["notes", "share", id] as const,
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
              pinned: filter.pinned || undefined,
              hidden: filter.hidden || undefined,
              kind: filter.kind,
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

/** 标签和数量。hidden 为 true 时是隐藏空间自己的标签。 */
export function useTags(hidden = false) {
  return useQuery({
    queryKey: [...notesKeys.tags, hidden],
    queryFn: () =>
      unwrap(
        notesApi.GET("/notes/tags", {
          params: { query: hidden ? { hidden: true } : {} },
        }),
      ),
  });
}

export function useSetTagColor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { tag: string; color: string }) =>
      unwrap(notesApi.PUT("/notes/tag-colors", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: notesKeys.tags }),
    onError: (error: unknown) =>
      toast({ message: errorMessage(error), tone: "error" }),
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
      qc.invalidateQueries({ queryKey: notesKeys.counts });
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

/** 附件最大 50 MB，和服务端一致。 */
export const MAX_ATTACHMENT_BYTES = 50 * 1024 * 1024;

/** 上传一个附件，返回附件信息。 */
export async function uploadAttachment(
  noteId: number,
  file: File,
): Promise<Attachment> {
  const form = new FormData();
  form.append("file", file, file.name || "image.png");
  const res = await apiFetch(`/notes/${noteId}/attachments`, {
    method: "POST",
    body: form,
  });
  return (await res.json()) as Attachment;
}

/** 命令面板的 "> 内容"：直接存成一条便签（B4、B73），AI 随后加标题和标签。 */
export async function captureNote(text: string) {
  const note = await unwrap(
    notesApi.POST("/notes", {
      body: { body: text, kind: "memo", quick: true },
    }),
  );
  queryClient.setQueryData(notesKeys.note(note.id), note);
  queryClient.invalidateQueries({ queryKey: notesKeys.lists });
  return note;
}

/* ---- B32：自动起标题、自动加标签 ---- */

export function useNoteAiSettings(enabled = true) {
  return useQuery({
    queryKey: notesKeys.aiSettings,
    queryFn: () => unwrap(notesApi.GET("/notes/ai-settings")),
    retry: (count, error) => !isNotLive(error) && count < 2,
    enabled,
  });
}

export function useSaveNoteAiSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NoteAiSettingsInput) =>
      unwrap(notesApi.PUT("/notes/ai-settings", { body })),
    onSuccess: (data) => qc.setQueryData(notesKeys.aiSettings, data),
    onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
  });
}

/** 不要这次的建议标签。 */
export function useDismissSuggestedTags() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        notesApi.DELETE("/notes/{noteId}/suggested-tags", {
          params: { path: { noteId: id } },
        }),
      ),
    onSuccess: (_d, id) =>
      qc.setQueryData<Note>(notesKeys.note(id), (note) =>
        note ? { ...note, suggestedTags: [] } : note,
      ),
    onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
  });
}

/* ---- B40：编辑器里手动点的 AI 功能，只返回结果，不保存 ---- */

export function useNoteAiTools() {
  const onError = (error: unknown) =>
    toast({ message: errorMessage(error), tone: "error" });
  return {
    polish: useMutation({
      mutationFn: ({ body, prompt }: { body: string; prompt?: string }) =>
        unwrap(notesApi.POST("/notes/ai/polish", { body: { body, prompt } })),
      onError,
    }),
    title: useMutation({
      mutationFn: (body: string) =>
        unwrap(notesApi.POST("/notes/ai/title", { body: { body } })),
      onError,
    }),
    tags: useMutation({
      mutationFn: (body: string) =>
        unwrap(notesApi.POST("/notes/ai/tags", { body: { body } })),
      onError,
    }),
  };
}

/* ---- B73：数量（左栏后面的数字）。回 404、501 表示便签还没上线 ---- */

export function useNoteCounts() {
  return useQuery({
    queryKey: notesKeys.counts,
    queryFn: () => unwrap(notesApi.GET("/notes/counts")),
    retry: (count, error) => !isNotLive(error) && count < 2,
    meta: { silentError: true },
  });
}

/* ---- B72：外链分享 ---- */

/** 这条笔记的外链。没分享时接口回 404，这里返回 null。 */
export function useNoteShare(id: number) {
  return useQuery({
    queryKey: notesKeys.share(id),
    queryFn: async () => {
      try {
        return await unwrap(
          notesApi.GET("/notes/{noteId}/share", {
            params: { path: { noteId: id } },
          }),
        );
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
    retry: (count, error) => !isNotLive(error) && count < 2,
  });
}

export function useSaveNoteShare(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NoteShareInput) =>
      withElevation(() =>
        unwrap(
          notesApi.PUT("/notes/{noteId}/share", {
            params: { path: { noteId: id } },
            body,
          }),
        ),
      ),
    onSuccess: (share) => {
      qc.setQueryData(notesKeys.share(id), share);
      qc.invalidateQueries({ queryKey: notesKeys.note(id) });
      qc.invalidateQueries({ queryKey: notesKeys.lists });
    },
  });
}

export function useDeleteNoteShare(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap(
        notesApi.DELETE("/notes/{noteId}/share", {
          params: { path: { noteId: id } },
        }),
      ),
    onSuccess: () => {
      qc.setQueryData(notesKeys.share(id), null);
      qc.invalidateQueries({ queryKey: notesKeys.note(id) });
      qc.invalidateQueries({ queryKey: notesKeys.lists });
    },
    onError: fail,
  });
}
