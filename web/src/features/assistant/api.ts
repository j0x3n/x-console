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
export type AiProvider = components["schemas"]["AiProvider"];
export type AiProviderInput = components["schemas"]["AiProviderInput"];
export type AiProviderPatch = components["schemas"]["AiProviderPatch"];
export type AiModel = components["schemas"]["AiModel"];
export type AiModelSettings = components["schemas"]["AiModelSettings"];
export type AiModelSettingsInput =
  components["schemas"]["AiModelSettingsInput"];
export type AiModelSpecInput = components["schemas"]["AiModelSpecInput"];
export type AiUsage = components["schemas"]["AiUsage"];
export type ModelRef = components["schemas"]["ModelRef"];
export type ReasoningEffort = components["schemas"]["ReasoningEffort"];
export type HostAgentPermission = components["schemas"]["HostAgentPermission"];

export const aiKeys = {
  all: ["ai"] as const,
  conversations: ["ai", "conversations"] as const,
  conversation: (id: number) => ["ai", "conversation", id] as const,
  tools: ["ai", "tools"] as const,
  providers: ["ai", "providers"] as const,
  models: ["ai", "models"] as const,
  modelSettings: ["ai", "model-settings"] as const,
  usage: (month: string) => ["ai", "usage", month] as const,
  hostConversations: (hostId: string) =>
    ["ai", "host-conversations", hostId] as const,
};

/** 后端还没做的接口不重试，其他的重试两次。 */
const retryUnlessNotLive = (count: number, error: unknown) =>
  !isNotLive(error) && count < 2;

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

/* ---- B32：OpenAI 兼容接口的供应商和模型 ---- */

/** 供应商列表。回 404 或 501 表示 B32 后端还没上线，设置页退回旧的表单。 */
export function useAiProviders(enabled = true) {
  return useQuery({
    queryKey: aiKeys.providers,
    queryFn: () => unwrap(aiApi.GET("/ai/providers")),
    retry: retryUnlessNotLive,
    enabled,
  });
}

export function useAiModels(enabled = true) {
  return useQuery({
    queryKey: aiKeys.models,
    queryFn: () => unwrap(aiApi.GET("/ai/models")),
    retry: retryUnlessNotLive,
    enabled,
  });
}

export function useModelSettings(enabled = true) {
  return useQuery({
    queryKey: aiKeys.modelSettings,
    queryFn: () => unwrap(aiApi.GET("/ai/model-settings")),
    retry: retryUnlessNotLive,
    enabled,
  });
}

export function useAiUsage(month: string, enabled = true) {
  return useQuery({
    queryKey: aiKeys.usage(month),
    queryFn: () =>
      unwrap(aiApi.GET("/ai/usage", { params: { query: { month } } })),
    retry: retryUnlessNotLive,
    enabled,
  });
}

/** 供应商的增删改。都要提升权限。 */
export function useProviderMutations() {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: aiKeys.all });
  const create = useMutation({
    mutationFn: (body: AiProviderInput) =>
      withElevation(() => unwrap(aiApi.POST("/ai/providers", { body }))),
    onSuccess: done,
  });
  const update = useMutation({
    mutationFn: ({ id, body }: { id: number; body: AiProviderPatch }) =>
      withElevation(() =>
        unwrap(
          aiApi.PATCH("/ai/providers/{providerId}", {
            params: { path: { providerId: id } },
            body,
          }),
        ),
      ),
    onSuccess: done,
  });
  const remove = useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          aiApi.DELETE("/ai/providers/{providerId}", {
            params: { path: { providerId: id } },
          }),
        ),
      ),
    onSuccess: done,
  });
  const test = useMutation({
    mutationFn: (id: number) =>
      unwrap(
        aiApi.POST("/ai/providers/{providerId}/test", {
          params: { path: { providerId: id } },
        }),
      ),
  });
  const refresh = useMutation({
    mutationFn: (id: number) =>
      unwrap(
        aiApi.POST("/ai/providers/{providerId}/models", {
          params: { path: { providerId: id } },
        }),
      ),
    onSuccess: done,
  });
  return { create, update, remove, test, refresh };
}

export function useSetModelSpec() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AiModelSpecInput) =>
      unwrap(aiApi.PUT("/ai/model-specs", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: aiKeys.models }),
  });
}

export function useSaveModelSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AiModelSettingsInput) =>
      withElevation(() => unwrap(aiApi.PUT("/ai/model-settings", { body }))),
    onSuccess: (data) => qc.setQueryData(aiKeys.modelSettings, data),
  });
}

/* ---- B33：机器的 Agent 会话 ---- */

export function useHostConversations(hostId: string) {
  return useQuery({
    queryKey: aiKeys.hostConversations(hostId),
    queryFn: () =>
      unwrap(
        aiApi.GET("/ai/host-agent/{hostId}/conversations", {
          params: { path: { hostId } },
        }),
      ),
    retry: retryUnlessNotLive,
  });
}

export function useCreateHostConversation(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap(
        aiApi.POST("/ai/host-agent/{hostId}/conversations", {
          params: { path: { hostId } },
          body: {},
        }),
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: aiKeys.hostConversations(hostId) }),
  });
}

/** 改会话的权限。“全部自动”要提升权限。 */
export function useSetPermission() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mode }: { id: number; mode: HostAgentPermission }) => {
      const call = () =>
        unwrap(
          aiApi.PUT("/ai/conversations/{conversationId}/permission", {
            params: { path: { conversationId: id } },
            body: { mode },
          }),
        );
      return mode === "all_auto" ? withElevation(call) : call();
    },
    onSettled: (_d, _e, { id }) =>
      qc.invalidateQueries({ queryKey: aiKeys.conversation(id) }),
  });
}
