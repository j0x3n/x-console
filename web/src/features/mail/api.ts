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

export interface MailFilter {
  accountId: number | null;
  unread: boolean;
  q: string;
}

export const mailKeys = {
  all: ["mail"] as const,
  accounts: ["mail", "accounts"] as const,
  list: (f: MailFilter) => ["mail", "list", f] as const,
  message: (id: number) => ["mail", "message", id] as const,
  summary: ["mail", "summary"] as const,
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
