import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/contacts";

/* 联系人和重要日期（B122）：生日、纪念日和上次联系的日期，到期前提醒。 */
export const contactsApi = createApi<paths>();

type S = components["schemas"];
export type ContactItem = S["Contact"];
export type ContactInput = S["ContactInput"];
export type ContactPatch = S["ContactPatch"];
export type ContactEvent = S["ContactEvent"];
export type ContactEventInput = S["ContactEventInput"];
export type ContactEventKind = S["ContactEventKind"];
export type ContactGroup = S["ContactGroup"];
export type ContactStatus = S["ContactStatus"];
export type ContactSummary = S["ContactSummary"];

export const contactKeys = {
  all: ["contacts"] as const,
  list: (archived: boolean) => ["contacts", "list", archived] as const,
};

// 服务端每次增删改发一次事件
invalidateOn("contact.", contactKeys.all);

/** 全部联系人。分组筛选和搜索在页面里做，数据量很小。 */
export function useContacts(archived: boolean) {
  return useQuery({
    queryKey: contactKeys.list(archived),
    queryFn: () =>
      unwrap(contactsApi.GET("/contacts", { params: { query: { archived } } })),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: contactKeys.all });
}

export function useCreateContact() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: ContactInput) =>
      unwrap(contactsApi.POST("/contacts", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateContact() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: ContactPatch }) =>
      unwrap(
        contactsApi.PATCH("/contacts/{contactId}", {
          params: { path: { contactId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

/** 记一次联系，日期不传就是今天。 */
export function useTouchContact() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; date?: string }) =>
      unwrap(
        contactsApi.POST("/contacts/{contactId}/touch", {
          params: { path: { contactId: v.id } },
          body: v.date ? { date: v.date } : {},
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteContact() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        contactsApi.DELETE("/contacts/{contactId}", {
          params: { path: { contactId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}
