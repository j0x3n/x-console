import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/assistant";

export const assistantApi = createApi<paths>();
type Schemas = components["schemas"];
export type Conversation = Schemas["Conversation"];
export type ConversationDetail = Schemas["ConversationDetail"];
export type PendingAction = Schemas["PendingAction"];

export const aiKeys = {
  all: ["assistant"] as const,
  conversations: ["assistant", "conversations"] as const,
  detail: (id: string) => ["assistant", "conversation", id] as const,
  settings: ["assistant", "settings"] as const,
};
invalidateOn("ai.message_done", aiKeys.all);
invalidateOn("ai.action_pending", aiKeys.all);
invalidateOn("ai.action_result", aiKeys.all);
invalidateOn("ai.error", aiKeys.all);

export function useConversations() {
  return useQuery({
    queryKey: aiKeys.conversations,
    queryFn: () => unwrap(assistantApi.GET("/ai/conversations")),
  });
}
export function useConversation(id: string) {
  return useQuery({
    queryKey: aiKeys.detail(id),
    queryFn: () =>
      unwrap(
        assistantApi.GET("/ai/conversations/{id}", {
          params: { path: { id } },
        }),
      ),
    enabled: !!id,
  });
}
export function useAISettings() {
  return useQuery({
    queryKey: aiKeys.settings,
    queryFn: () => unwrap(assistantApi.GET("/ai/settings")),
  });
}
export function useCreateConversation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (title?: string) =>
      unwrap(assistantApi.POST("/ai/conversations", { body: { title } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: aiKeys.conversations }),
  });
}
export function useSendMessage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, text }: { id: string; text: string }) =>
      unwrap(
        assistantApi.POST("/ai/conversations/{id}/messages", {
          params: { path: { id } },
          body: { text },
        }),
      ),
    onSuccess: (_, { id }) =>
      qc.invalidateQueries({ queryKey: aiKeys.detail(id) }),
  });
}
export function useDecideAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, approved }: { id: string; approved: boolean }) =>
      approved
        ? unwrap(
            assistantApi.POST("/ai/actions/{id}/approve", {
              params: { path: { id } },
            }),
          )
        : unwrap(
            assistantApi.POST("/ai/actions/{id}/reject", {
              params: { path: { id } },
            }),
          ),
    onSuccess: () => qc.invalidateQueries({ queryKey: aiKeys.all }),
  });
}
export function useSaveAISettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["UpdateAISettings"]) =>
      unwrap(assistantApi.PUT("/ai/settings", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: aiKeys.settings }),
  });
}
