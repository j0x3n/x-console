import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import type { components, paths } from "../../api/gen/mcp";

export const mcpApi = createApi<paths>();

export type ApiToken = components["schemas"]["ApiToken"];
export type ApiTokenAccess = components["schemas"]["ApiTokenAccess"];

export const mcpKeys = {
  all: ["mcp"] as const,
  tokens: ["mcp", "tokens"] as const,
  calls: ["mcp", "calls"] as const,
  tools: (access: ApiTokenAccess, modules: string[]) =>
    ["mcp", "tools", access, modules.join(",")] as const,
};

export function useTokens() {
  return useQuery({
    queryKey: mcpKeys.tokens,
    queryFn: () => unwrap(mcpApi.GET("/api-tokens")),
  });
}

export function useCalls() {
  return useQuery({
    queryKey: mcpKeys.calls,
    queryFn: () => unwrap(mcpApi.GET("/api-tokens/calls")),
    refetchInterval: 30_000,
  });
}

export function useTools(access: ApiTokenAccess, modules: string[]) {
  return useQuery({
    queryKey: mcpKeys.tools(access, modules),
    queryFn: () =>
      unwrap(
        mcpApi.GET("/api-tokens/tools", {
          params: {
            query: { access, modules: modules.length ? modules : undefined },
          },
        }),
      ),
  });
}

export function useTokenMutations() {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: mcpKeys.all });
  return {
    create: useMutation({
      mutationFn: (body: {
        name: string;
        access: ApiTokenAccess;
        modules: string[];
        expiresInDays?: number;
      }) => withElevation(() => unwrap(mcpApi.POST("/api-tokens", { body }))),
      onSuccess: done,
    }),
    revoke: useMutation({
      mutationFn: (id: number) =>
        withElevation(() =>
          unwrap(
            mcpApi.DELETE("/api-tokens/{tokenId}", {
              params: { path: { tokenId: id } },
            }),
          ),
        ),
      onSuccess: done,
    }),
  };
}
