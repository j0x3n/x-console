import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/ai";
import { withElevation } from "../../auth/elevation";

export const aiApi = createApi<paths>();

export type Conversation = components["schemas"]["Conversation"];
export type ConversationDetail = components["schemas"]["ConversationDetail"];
export type Message = components["schemas"]["Message"];
export type ContentBlock = components["schemas"]["ContentBlock"];
export type PendingAction = components["schemas"]["PendingAction"];
export type Tool = components["schemas"]["Tool"];
export type AiSettings = components["schemas"]["AiSettings"];

export const aiKeys = {
  all: ["ai"] as const,
  conversations: ["ai", "conversations"] as const,
  conversation: (id: number) => ["ai", "conversation", id] as const,
  tools: ["ai", "tools"] as const,
  settings: ["ai", "settings"] as const,
};

/** 服务端还没有助手接口（404 或 501）。只用在对话列表和设置上判断。 */
export function isNotLive(error: unknown) {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  );
}

export function useConversations(enabled = true) {
  return useQuery({
    queryKey: aiKeys.conversations,
    queryFn: () => unwrap(aiApi.GET("/ai/conversations")),
    enabled,
  });
}

export function useConversation(id: number | null) {
  return useQuery({
    queryKey: aiKeys.conversation(id ?? 0),
    queryFn: () =>
      unwrap(
        aiApi.GET("/ai/conversations/{conversationId}", {
          params: { path: { conversationId: id! } },
        }),
      ),
    enabled: id != null,
  });
}

export function useTools(enabled = true) {
  return useQuery({
    queryKey: aiKeys.tools,
    queryFn: () => unwrap(aiApi.GET("/ai/tools")),
    staleTime: 5 * 60_000,
    enabled,
  });
}

export function useAiSettings(enabled = true) {
  return useQuery({
    queryKey: aiKeys.settings,
    queryFn: () => unwrap(aiApi.GET("/ai/settings")),
    enabled,
  });
}

export function useSaveAiSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      apiKey?: string;
      model?: string;
      confirmAllWrites?: boolean;
    }) => withElevation(() => unwrap(aiApi.PUT("/ai/settings", { body }))),
    onSuccess: (data) => qc.setQueryData(aiKeys.settings, data),
  });
}

export interface PageContext {
  path: string;
  title: string;
}

/** 没有对话时先建一个，再发消息。返回对话 id。 */
export function useSendMessage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      conversationId,
      text,
      context,
    }: {
      conversationId: number | null;
      text: string;
      context?: PageContext;
    }) => {
      let id = conversationId;
      if (id == null) {
        const created = await unwrap(
          aiApi.POST("/ai/conversations", { body: {} }),
        );
        id = created.id;
      }
      await unwrap(
        aiApi.POST("/ai/conversations/{conversationId}/messages", {
          params: { path: { conversationId: id } },
          body: { text, context },
        }),
      );
      return id;
    },
    onSuccess: (id) => {
      qc.invalidateQueries({ queryKey: aiKeys.conversation(id) });
      qc.invalidateQueries({ queryKey: aiKeys.conversations });
    },
  });
}

export function useStopReply() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        aiApi.POST("/ai/conversations/{conversationId}/stop", {
          params: { path: { conversationId: id } },
        }),
      ),
    onSettled: (_d, _e, id) =>
      qc.invalidateQueries({ queryKey: aiKeys.conversation(id) }),
  });
}

export function useDeleteConversation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        aiApi.DELETE("/ai/conversations/{conversationId}", {
          params: { path: { conversationId: id } },
        }),
      ),
    onSuccess: (_d, id) => {
      qc.removeQueries({ queryKey: aiKeys.conversation(id) });
      qc.invalidateQueries({ queryKey: aiKeys.conversations });
    },
  });
}

/** 确认或拒绝一个动作。dangerous 动作确认时要提升权限。 */
export function useDecideAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      action,
      approve,
    }: {
      action: PendingAction;
      approve: boolean;
    }) => {
      const params = { params: { path: { actionId: action.id } } };
      return approve
        ? withElevation(() =>
            unwrap(aiApi.POST("/ai/actions/{actionId}/approve", params)),
          )
        : unwrap(aiApi.POST("/ai/actions/{actionId}/reject", params));
    },
    onSettled: (_d, _e, { action }) =>
      qc.invalidateQueries({
        queryKey: aiKeys.conversation(action.conversationId),
      }),
  });
}
