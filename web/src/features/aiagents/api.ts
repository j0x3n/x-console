import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { useCallback } from "react";
import {
  invalidateOn,
  useServerEvent,
  type ServerEvent,
} from "../../api/events";
import { withElevation } from "../../auth/elevation";
import "./i18n";
import type { components, paths } from "../../api/gen/aiagents";

export const agentsApi = createApi<paths>();

type S = components["schemas"];
export type AiAgent = S["AiAgent"];
export type AiAgentInput = S["AiAgentInput"];
export type AiAgentKind = S["AiAgentKind"];
export type GitConnection = S["GitConnection"];
export type RemoteRepo = S["RemoteRepo"];
export type AiAgentRun = S["AiAgentRun"];
export type AiAgentRunEvent = S["AiAgentRunEvent"];
export type AiAgentDecision = S["AiAgentDecision"];
export type AiAgentNotify = S["AiAgentNotify"];

export const agentKeys = {
  all: ["aiagents"] as const,
  agents: ["aiagents", "agents"] as const,
  agent: (id: number) => ["aiagents", "agents", id] as const,
  connections: ["aiagents", "connections"] as const,
  remoteRepos: (id: number, q: string) =>
    ["aiagents", "remote-repos", id, q] as const,
  runs: ["aiagents", "runs"] as const,
  runList: (filter: { agentId?: number; issueKey?: string }) =>
    ["aiagents", "runs", filter] as const,
  runEvents: (id: number) => ["aiagents", "run-events", id] as const,
  decisions: ["aiagents", "decisions"] as const,
  notify: ["aiagents", "notify"] as const,
};

invalidateOn("ai_agent.", agentKeys.agents);
invalidateOn("coding_task.", agentKeys.agents);
invalidateOn("git_connection.", agentKeys.connections);
// B86、B87：执行记录和等你决定的事
invalidateOn("ai_agent.", agentKeys.runs);
invalidateOn("coding_task.", agentKeys.runs);
invalidateOn("ai_agent.decision", agentKeys.decisions);
invalidateOn("ai.action", agentKeys.decisions);

export function useAiAgents() {
  return useQuery({
    queryKey: agentKeys.agents,
    queryFn: () => unwrap(agentsApi.GET("/ai-agents")),
  });
}

export function useAiAgent(id: number) {
  return useQuery({
    queryKey: agentKeys.agent(id),
    queryFn: () =>
      unwrap(
        agentsApi.GET("/ai-agents/{agentId}", {
          params: { path: { agentId: id } },
        }),
      ),
  });
}

export function useAgentMutations() {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: agentKeys.agents });
  return {
    create: useMutation({
      mutationFn: (body: AiAgentInput) =>
        withElevation(() => unwrap(agentsApi.POST("/ai-agents", { body }))),
      onSuccess: done,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: number; body: AiAgentInput }) =>
        withElevation(() =>
          unwrap(
            agentsApi.PATCH("/ai-agents/{agentId}", {
              params: { path: { agentId: id } },
              body,
            }),
          ),
        ),
      onSuccess: done,
    }),
    remove: useMutation({
      mutationFn: (id: number) =>
        unwrap(
          agentsApi.DELETE("/ai-agents/{agentId}", {
            params: { path: { agentId: id } },
          }),
        ),
      onSuccess: done,
    }),
  };
}

export interface AssignInput {
  agentId: number;
  issueKey: string;
  repoId?: number;
  baseBranch?: string;
  runnerAgentId?: string;
  note?: string;
  openPr?: boolean;
}

export function useAssignAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, ...body }: AssignInput) =>
      unwrap(
        agentsApi.POST("/ai-agents/{agentId}/assign", {
          params: { path: { agentId } },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: agentKeys.agents }),
  });
}

export function useConnections() {
  return useQuery({
    queryKey: agentKeys.connections,
    queryFn: () => unwrap(agentsApi.GET("/git-connections")),
  });
}

export function useRemoteRepos(id: number | undefined, q: string) {
  return useQuery({
    queryKey: agentKeys.remoteRepos(id ?? 0, q),
    enabled: !!id,
    queryFn: () =>
      unwrap(
        agentsApi.GET("/git-connections/{connectionId}/repos", {
          params: { path: { connectionId: id! }, query: q ? { q } : {} },
        }),
      ),
  });
}

export interface NewConnection {
  kind: "github" | "forgejo";
  name: string;
  baseUrl?: string;
  token?: string;
  useGithubModule?: boolean;
}

export function useConnectionMutations() {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: agentKeys.connections });
  return {
    create: useMutation({
      mutationFn: (body: NewConnection) =>
        withElevation(() =>
          unwrap(agentsApi.POST("/git-connections", { body })),
        ),
      onSuccess: done,
    }),
    update: useMutation({
      mutationFn: ({
        id,
        body,
      }: {
        id: number;
        body: { name?: string; token?: string };
      }) =>
        withElevation(() =>
          unwrap(
            agentsApi.PATCH("/git-connections/{connectionId}", {
              params: { path: { connectionId: id } },
              body,
            }),
          ),
        ),
      onSuccess: done,
    }),
    check: useMutation({
      mutationFn: (id: number) =>
        unwrap(
          agentsApi.POST("/git-connections/{connectionId}/check", {
            params: { path: { connectionId: id } },
          }),
        ),
      onSuccess: done,
    }),
    remove: useMutation({
      mutationFn: (id: number) =>
        withElevation(() =>
          unwrap(
            agentsApi.DELETE("/git-connections/{connectionId}", {
              params: { path: { connectionId: id } },
            }),
          ),
        ),
      onSuccess: done,
    }),
    webhook: useMutation({
      mutationFn: (id: number) =>
        withElevation(() =>
          unwrap(
            agentsApi.GET("/git-connections/{connectionId}/webhook", {
              params: { path: { connectionId: id } },
            }),
          ),
        ),
    }),
  };
}

/**
 * B86：Agent 的执行记录。后端还没上线时回 404 或 501，
 * 调用方用 isNotLive 判断，退回到只看编码任务。
 */
export function useAgentRuns(filter: { agentId?: number; issueKey?: string }) {
  return useQuery({
    queryKey: agentKeys.runList(filter),
    queryFn: () =>
      unwrap(
        agentsApi.GET("/ai-agents/runs", {
          params: {
            query: {
              agentId: filter.agentId,
              issueKey: filter.issueKey,
              limit: 50,
            },
          },
        }),
      ),
    retry: false,
    meta: { silentError: true },
  });
}

/** B86：内置 Agent 一次执行的日志。执行中收到 ai_agent.run_event 就重新取。 */
export function useAgentRunEvents(runId: number | undefined) {
  const qc = useQueryClient();
  const id = runId ?? 0;
  useServerEvent(
    "ai_agent.run_event",
    useCallback(
      (event: ServerEvent) => {
        const data = event.data as { runId?: number } | undefined;
        if (data?.runId === id)
          qc.invalidateQueries({ queryKey: agentKeys.runEvents(id) });
      },
      [id, qc],
    ),
  );
  return useQuery({
    queryKey: agentKeys.runEvents(id),
    queryFn: () =>
      unwrap(
        agentsApi.GET("/ai-agents/runs/{runId}/events", {
          params: { path: { runId: id }, query: {} },
        }),
      ),
    enabled: id > 0,
    retry: false,
  });
}

export function useCancelAgentRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (runId: number) =>
      unwrap(
        agentsApi.POST("/ai-agents/runs/{runId}/cancel", {
          params: { path: { runId } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: agentKeys.runs }),
  });
}

/** B87：等你决定的事（权限请求、问题）。没上线时当成空列表。 */
export function useAgentDecisions() {
  return useQuery({
    queryKey: agentKeys.decisions,
    queryFn: () => unwrap(agentsApi.GET("/ai-agents/decisions")),
    retry: false,
    meta: { silentError: true },
  });
}

export function useAnswerDecision() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...body
    }: {
      id: string;
      approve?: boolean;
      answer?: string;
    }) =>
      unwrap(
        agentsApi.POST("/ai-agents/decisions/{decisionId}", {
          params: { path: { decisionId: id } },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: agentKeys.decisions }),
  });
}

/** B87：Agent 通知的六个开关。 */
export function useAgentNotify() {
  return useQuery({
    queryKey: agentKeys.notify,
    queryFn: () => unwrap(agentsApi.GET("/ai-agents/notify")),
    retry: false,
  });
}

export function useSaveAgentNotify() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AiAgentNotify) =>
      unwrap(agentsApi.PUT("/ai-agents/notify", { body })),
    onSuccess: (data) => qc.setQueryData(agentKeys.notify, data),
  });
}
