import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "./client";
import { invalidateOn } from "./events";
import type { components, paths } from "./gen/core";

export const coreApi = createApi<paths>();

export type AuthStatus = components["schemas"]["AuthStatus"];
export type Agent = components["schemas"]["Agent"];
export type AgentKind = components["schemas"]["AgentKind"];
export type AuditEntry = components["schemas"]["AuditEntry"];
export type AppNotification = components["schemas"]["Notification"];

export const coreKeys = {
  auth: ["auth", "status"] as const,
  agents: ["agents"] as const,
  audit: ["audit"] as const,
  notifications: ["notifications"] as const,
};

invalidateOn("agent.", coreKeys.agents);
invalidateOn("notification.", coreKeys.notifications);

export function useAuthStatus() {
  return useQuery({
    queryKey: coreKeys.auth,
    queryFn: () => unwrap(coreApi.GET("/auth/status")),
    staleTime: 60_000,
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(coreApi.POST("/auth/logout")),
    onSuccess: () => {
      qc.clear();
      qc.invalidateQueries({ queryKey: coreKeys.auth });
    },
  });
}

export function useAgents(kind?: AgentKind) {
  return useQuery({
    queryKey: coreKeys.agents,
    queryFn: () => unwrap(coreApi.GET("/agents")),
    select: (agents) => (kind ? agents.filter((a) => a.kind === kind) : agents),
  });
}

export function useNotifications() {
  return useQuery({
    queryKey: [...coreKeys.notifications, "latest"],
    queryFn: () =>
      unwrap(
        coreApi.GET("/notifications", { params: { query: { limit: 30 } } }),
      ),
  });
}

export function useMarkNotificationRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        coreApi.POST("/notifications/{notificationId}/read", {
          params: { path: { notificationId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: coreKeys.notifications }),
  });
}

export function useMarkAllNotificationsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(coreApi.POST("/notifications/read-all")),
    onSuccess: () => qc.invalidateQueries({ queryKey: coreKeys.notifications }),
  });
}
