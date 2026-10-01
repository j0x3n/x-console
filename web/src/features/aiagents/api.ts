import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
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

export const agentKeys = {
  all: ["aiagents"] as const,
  agents: ["aiagents", "agents"] as const,
  agent: (id: number) => ["aiagents", "agents", id] as const,
  connections: ["aiagents", "connections"] as const,
  remoteRepos: (id: number, q: string) =>
    ["aiagents", "remote-repos", id, q] as const,
};

invalidateOn("ai_agent.", agentKeys.agents);
invalidateOn("coding_task.", agentKeys.agents);
invalidateOn("git_connection.", agentKeys.connections);

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
