import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { withElevation } from "../../auth/elevation";
import type { components, paths } from "../../api/gen/mail";

export const mailApi = createApi<paths>();

type S = components["schemas"];
export type MailAccount = S["MailAccount"];
export type MailAccountInput = S["MailAccountInput"];
export type MailAccountPatch = S["MailAccountPatch"];
export type MailProvider = S["MailProvider"];
export type MailSummary = S["MailSummary"];
export type MailMessage = S["MailMessage"];
export type MailSummaryView = S["MailSummaryView"];
export type MailTriage = S["MailTriage"];
export type MailCategory = S["MailCategory"];
export type MailBatchAction = S["MailBatchAction"];
export type MailBatchResult = S["MailBatchResult"];

export interface MailFilter {
  accountId: number | null;
  unread: boolean;
  q: string;
  /** B136：只看 AI 分拣成这一类的邮件 */
  category?: MailCategory | null;
}

export const mailKeys = {
  all: ["mail"] as const,
  accounts: ["mail", "accounts"] as const,
  list: (f: MailFilter) => ["mail", "list", f] as const,
  message: (id: number) => ["mail", "message", id] as const,
  summary: ["mail", "summary"] as const,
  todo: ["mail", "todo"] as const,
};

// 新邮件、已读状态、账号连接状态变化时服务端发 mail.*
invalidateOn("mail.", mailKeys.all);

export function useMailAccounts() {
  return useQuery({
    queryKey: mailKeys.accounts,
    queryFn: () => unwrap(mailApi.GET("/mail/accounts")),
    retry: false,
  });
}

export function useSaveMailAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      create,
      patch,
    }: {
      id?: number;
      create?: MailAccountInput;
      patch?: MailAccountPatch;
    }) =>
      withElevation(() =>
        id
          ? unwrap(
              mailApi.PATCH("/mail/accounts/{accountId}", {
                params: { path: { accountId: id } },
                body: patch ?? {},
              }),
            )
          : unwrap(mailApi.POST("/mail/accounts", { body: create! })),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

export function useDeleteMailAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          mailApi.DELETE("/mail/accounts/{accountId}", {
            params: { path: { accountId: id } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

export function useSyncMailAccount() {
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        mailApi.POST("/mail/accounts/{accountId}/sync", {
          params: { path: { accountId: id } },
        }),
      ),
  });
}

export function useMailMessages(filter: MailFilter, enabled = true) {
  return useInfiniteQuery({
    queryKey: mailKeys.list(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        mailApi.GET("/mail/messages", {
          params: {
            query: {
              accountId: filter.accountId ?? undefined,
              unread: filter.unread || undefined,
              q: filter.q || undefined,
              category: filter.category || undefined,
              before: pageParam || undefined,
              limit: 50,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.nextBefore,
    placeholderData: (prev) => prev,
    enabled,
  });
}

export function useMailMessage(id: number | null) {
  return useQuery({
    queryKey: mailKeys.message(id ?? 0),
    queryFn: () =>
      unwrap(
        mailApi.GET("/mail/messages/{messageId}", {
          params: { path: { messageId: id! } },
        }),
      ),
    enabled: id !== null,
    staleTime: 5 * 60_000,
  });
}

export function useUpdateMailMessage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      unread,
      flagged,
    }: {
      id: number;
      unread?: boolean;
      flagged?: boolean;
    }) =>
      unwrap(
        mailApi.PATCH("/mail/messages/{messageId}", {
          params: { path: { messageId: id } },
          body: { unread, flagged },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

/** B136：让 AI 分拣一封邮件（用正文，没有正文时用摘要）。 */
export function useTriageMailMessage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        mailApi.POST("/mail/messages/{messageId}/triage", {
          params: { path: { messageId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

/** B219：批量标已读、未读、星标或删除。 */
export function useBatchMail() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ids, action }: { ids: number[]; action: MailBatchAction }) =>
      unwrap(mailApi.POST("/mail/messages/batch", { body: { ids, action } })),
    onSettled: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

/** B219：一个邮箱（不传就是全部邮箱）的未读全部标成已读。 */
export function useMarkAllMailRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (accountId: number | null) =>
      unwrap(
        mailApi.POST("/mail/read-all", {
          body: { accountId: accountId ?? undefined },
        }),
      ),
    onSettled: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

/** B220：今日页用。AI 分拣成要处理、还没标成已处理的邮件。 */
export function useMailTodo(enabled = true) {
  return useQuery({
    queryKey: mailKeys.todo,
    queryFn: async () => (await unwrap(mailApi.GET("/mail/todo"))).items ?? [],
    retry: false,
    enabled,
    meta: { silentError: true },
  });
}

/** B220：把一封要处理的邮件标成已处理，今日页不再显示。 */
export function useSetMailDone() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, done }: { id: number; done: boolean }) =>
      unwrap(
        mailApi.PUT("/mail/messages/{messageId}/done", {
          params: { path: { messageId: id } },
          body: { done },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mailKeys.all }),
  });
}

/** 今日页用的未读数和最近几封。 */
export function useMailSummary(enabled = true) {
  return useQuery({
    queryKey: mailKeys.summary,
    // 后端没有邮箱时 accounts、latest 是 null，这里统一成空数组
    queryFn: async () => {
      const s = await unwrap(mailApi.GET("/mail/summary"));
      return { ...s, accounts: s.accounts ?? [], latest: s.latest ?? [] };
    },
    retry: false,
    enabled,
    meta: { silentError: true },
  });
}

/** 附件和内嵌图片的地址。 */
export function attachmentUrl(messageId: number, index: number) {
  return `/api/v1/mail/messages/${messageId}/attachments/${index}`;
}
