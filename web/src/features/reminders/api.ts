import { useMutation, useQuery } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
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
};

export const notifyKeys = {
  all: ["notify"] as const,
  channels: ["notify", "channels"] as const,
  routes: ["notify", "routes"] as const,
  quiet: ["notify", "quiet-hours"] as const,
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
