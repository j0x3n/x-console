import { useMutation, useQuery } from "@tanstack/react-query";
import { createApi, isNotLive, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { useInvalidate } from "../../api/useInvalidate";
import type { components, paths } from "../../api/gen/reminders";
import { readShowExternal } from "./external";

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
  mutes: ["notify", "mutes"] as const,
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

/**
 * 今天和即将到来的数量，和提醒页标签上的数一致：自己的提醒加上其他模块的
 * 到期事项（提醒页“显示其他模块”关掉时不算）。二级菜单和今日页用。
 */
export function useReminderCounts() {
  const showExternal = readShowExternal();
  const today = useReminders("today");
  const upcoming = useReminders("upcoming");
  const extToday = useExternalReminders("today", showExternal);
  const extUpcoming = useExternalReminders("upcoming", showExternal);
  const open = (rs: Reminder[] | undefined) =>
    (rs ?? []).filter((r) => r.status !== "done" && r.status !== "ended");
  const openExt = (xs: ExternalReminder[] | undefined) =>
    showExternal ? (xs ?? []).filter((x) => !x.done) : [];
  return {
    loading: today.isPending || upcoming.isPending,
    error: today.isError && upcoming.isError,
    today: open(today.data).length + openExt(extToday.data).length,
    upcoming: open(upcoming.data).length + openExt(extUpcoming.data).length,
    /** 今天其他模块的到期事项，今日页“今天要做”里列出来 */
    externalToday: openExt(extToday.data),
  };
}

// ---- 静音规则（B113）----

export type NotifyMute = Schemas["NotifyMute"];

/** 静音规则。传了 scope 只看这个范围的，比如 mail:3。 */
export function useNotifyMutes(scope?: string, enabled = true) {
  return useQuery({
    queryKey: [...notifyKeys.mutes, scope ?? "all"],
    queryFn: async () =>
      (
        await unwrap(
          remindersApi.GET("/notify/mutes", {
            params: { query: scope ? { scope } : {} },
          }),
        )
      ).items,
    enabled,
    retry: (count, error) => !isNotLive(error) && count < 2,
  });
}

export function useCreateNotifyMute() {
  const invalidate = useInvalidate(notifyKeys.mutes);
  return useMutation({
    mutationFn: (body: {
      kindPattern: string;
      scope?: string;
      target: string;
    }) => unwrap(remindersApi.POST("/notify/mutes", { body })),
    onSuccess: invalidate,
  });
}

export function useDeleteNotifyMute() {
  const invalidate = useInvalidate(notifyKeys.mutes);
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        remindersApi.DELETE("/notify/mutes/{muteId}", {
          params: { path: { muteId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

/** 把一个范围里被静音的目标整个换成 targets（邮箱编辑弹窗保存时用）。 */
export function useReplaceScopeMutes() {
  const invalidate = useInvalidate(notifyKeys.mutes);
  return useMutation({
    mutationFn: (body: {
      kindPattern: string;
      scope: string;
      targets: string[];
    }) => unwrap(remindersApi.PUT("/notify/mutes/scope", { body })),
    onSuccess: invalidate,
  });
}
