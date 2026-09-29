import { useMutation, useQuery } from "@tanstack/react-query";
import { createApi, isNotLive, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { useInvalidate } from "../../api/useInvalidate";
import type { components, paths } from "../../api/gen/reminders";

export const remindersApi = createApi<paths>();

type Schemas = components["schemas"];
export type Reminder = Schemas["Reminder"];
export type ReminderInput = Schemas["ReminderInput"];
export type ReminderPatch = Schemas["ReminderPatch"];
export type ReminderStatus = Schemas["ReminderStatus"];
export type NotifyChannel = Schemas["NotifyChannel"];
export type ChannelField = Schemas["ChannelField"];
export type NotifyRoute = Schemas["NotifyRoute"];
export type NotifyPriority = Schemas["NotifyPriority"];
export type QuietHours = Schemas["QuietHours"];
export type ChannelName = "webpush" | "telegram" | "bark" | "serverchan";
export type ReminderRange = "today" | "upcoming" | "done";

export const reminderKeys = {
  all: ["reminders"] as const,
  list: (range: ReminderRange) => ["reminders", "list", range] as const,
  external: (range: "today" | "upcoming") =>
    ["reminders", "external", range] as const,
};

export const notifyKeys = {
  all: ["notify"] as const,
  channels: ["notify", "channels"] as const,
  routes: ["notify", "routes"] as const,
  quiet: ["notify", "quiet-hours"] as const,
  subscriptions: ["notify", "webpush-subscriptions"] as const,
};

invalidateOn("reminder.", reminderKeys.all);
invalidateOn("notify.", notifyKeys.all);

export function useReminders(range: ReminderRange) {
  return useQuery({
    queryKey: reminderKeys.list(range),
    queryFn: () =>
      unwrap(remindersApi.GET("/reminders", { params: { query: { range } } })),
    select: (data) => data.items,
  });
}

export function useCreateReminder() {
  const invalidate = useInvalidate(reminderKeys.all);
  return useMutation({
    mutationFn: (body: ReminderInput) =>
      unwrap(remindersApi.POST("/reminders", { body })),
    onSuccess: invalidate,
  });
}

export function useUpdateReminder() {
  const invalidate = useInvalidate(reminderKeys.all);
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: ReminderPatch }) =>
      unwrap(
        remindersApi.PATCH("/reminders/{reminderId}", {
          params: { path: { reminderId: id } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useCompleteReminder() {
  const invalidate = useInvalidate(reminderKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        remindersApi.POST("/reminders/{reminderId}/done", {
          params: { path: { reminderId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useSnoozeReminder() {
  const invalidate = useInvalidate(reminderKeys.all);
  return useMutation({
    mutationFn: ({ id, minutes }: { id: number; minutes: number }) =>
      unwrap(
        remindersApi.POST("/reminders/{reminderId}/snooze", {
          params: { path: { reminderId: id } },
          body: { minutes },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteReminder() {
  const invalidate = useInvalidate(reminderKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        remindersApi.DELETE("/reminders/{reminderId}", {
          params: { path: { reminderId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

// ---- 通知设置 ----

export function useNotifyChannels() {
  return useQuery({
    queryKey: notifyKeys.channels,
    queryFn: () => unwrap(remindersApi.GET("/notify/channels")),
  });
}

export function useUpdateChannel() {
  const invalidate = useInvalidate(notifyKeys.channels);
  return useMutation({
    mutationFn: ({
      channel,
      values,
    }: {
      channel: ChannelName;
      values: Record<string, string>;
    }) =>
      unwrap(
        remindersApi.PUT("/notify/channels/{channel}", {
          params: { path: { channel } },
          body: { values },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useTestChannel() {
  return useMutation({
    mutationFn: (channel: ChannelName) =>
      unwrap(
        remindersApi.POST("/notify/channels/{channel}/test", {
          params: { path: { channel } },
        }),
      ),
  });
}

export function useRegisterTelegramWebhook() {
  const invalidate = useInvalidate(notifyKeys.channels);
  return useMutation({
    mutationFn: () =>
      unwrap(remindersApi.POST("/notify/telegram/register-webhook")),
    onSuccess: invalidate,
  });
}

export function useNotifyRoutes() {
  return useQuery({
    queryKey: notifyKeys.routes,
    queryFn: () => unwrap(remindersApi.GET("/notify/routes")),
  });
}

export function useSaveRoutes() {
  const invalidate = useInvalidate(notifyKeys.routes);
  return useMutation({
    mutationFn: (routes: NotifyRoute[]) =>
      unwrap(remindersApi.PUT("/notify/routes", { body: routes })),
    onSuccess: invalidate,
  });
}

export function useQuietHours() {
  return useQuery({
    queryKey: notifyKeys.quiet,
    queryFn: () => unwrap(remindersApi.GET("/notify/quiet-hours")),
  });
}

export function useSaveQuietHours() {
  const invalidate = useInvalidate(notifyKeys.quiet);
  return useMutation({
    mutationFn: (body: QuietHours) =>
      unwrap(remindersApi.PUT("/notify/quiet-hours", { body })),
    onSuccess: invalidate,
  });
}

/* ---- B34：浏览器推送的订阅列表和测试 ---- */

export type WebPushSubscriptionInfo =
  components["schemas"]["WebPushSubscriptionInfo"];
export type WebPushTestResult = components["schemas"]["WebPushTestResult"];

/** 订阅列表。回 404 或 501 表示后端还没做，卡片只显示订阅数量。 */
export function usePushSubscriptions() {
  return useQuery({
    queryKey: notifyKeys.subscriptions,
    queryFn: () => unwrap(remindersApi.GET("/notify/webpush/subscriptions")),
    retry: (count, error) => !isNotLive(error) && count < 2,
  });
}

export function useDeletePushSubscription() {
  const invalidate = useInvalidate(notifyKeys.all);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        remindersApi.DELETE("/notify/webpush/subscriptions/{subscriptionId}", {
          params: { path: { subscriptionId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

/**
 * 服务端推送测试。新接口没上线时退回旧的渠道测试，结果为 null。
 */
export function useTestWebPush() {
  const invalidate = useInvalidate(notifyKeys.subscriptions);
  return useMutation({
    mutationFn: async (): Promise<WebPushTestResult[] | null> => {
      try {
        return await unwrap(remindersApi.POST("/notify/webpush/test"));
      } catch (err) {
        if (!isNotLive(err)) throw err;
        await unwrap(
          remindersApi.POST("/notify/channels/{channel}/test", {
            params: { path: { channel: "webpush" } },
          }),
        );
        return null;
      }
    },
    onSettled: invalidate,
  });
}

/* ---- B37：其他模块的到期事项 ---- */

export type ExternalReminder = components["schemas"]["ExternalReminder"];

/** 回 404 或 501 表示后端还没做，页面当作没有。 */
export function useExternalReminders(
  range: "today" | "upcoming",
  enabled: boolean,
) {
  return useQuery({
    queryKey: reminderKeys.external(range),
    queryFn: () =>
      unwrap(
        remindersApi.GET("/reminders/external", {
          params: { query: { range } },
        }),
      ).then((r) => r.items),
    retry: (count, error) => !isNotLive(error) && count < 2,
    staleTime: 60_000,
    enabled,
  });
}
