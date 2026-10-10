import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/credentials";
import { withElevation } from "../../auth/elevation";

/* 密钥和令牌台账（B120）：只记信息，不存密钥本身。 */
export const credentialsApi = createApi<paths>();

type S = components["schemas"];
export type CredentialItem = S["Credential"];
export type CredentialInput = S["CredentialInput"];
export type CredentialPatch = S["CredentialPatch"];
export type CredentialRotate = S["CredentialRotate"];
export type CredentialKind = S["CredentialKind"];
export type CredentialStatus = S["CredentialStatus"];
export type CredentialSummary = S["CredentialSummary"];

export const credentialKeys = {
  all: ["credentials"] as const,
  list: (archived: boolean) => ["credentials", "list", archived] as const,
};

// 服务端每次增删改发一次事件
invalidateOn("credential.", credentialKeys.all);

/** 全部条目。类型筛选和搜索在页面里做，数据量很小。 */
export function useCredentials(archived: boolean) {
  return useQuery({
    queryKey: credentialKeys.list(archived),
    queryFn: () =>
      unwrap(
        credentialsApi.GET("/credentials", { params: { query: { archived } } }),
      ),
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: credentialKeys.all });
}

export function useCreateCredential() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: CredentialInput) =>
      unwrap(credentialsApi.POST("/credentials", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateCredential() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: CredentialPatch }) =>
      unwrap(
        credentialsApi.PATCH("/credentials/{credentialId}", {
          params: { path: { credentialId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useRotateCredential() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: CredentialRotate }) =>
      unwrap(
        credentialsApi.POST("/credentials/{credentialId}/rotate", {
          params: { path: { credentialId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteCredential() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      withElevation(() =>
        unwrap(
          credentialsApi.DELETE("/credentials/{credentialId}", {
            params: { path: { credentialId: id } },
          }),
        ),
      ),
    onSuccess: refresh,
  });
}
