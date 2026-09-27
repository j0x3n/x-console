import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/vault";
import { markVaultSession } from "./logic";

export const vaultApi = createApi<paths>();

export type VaultStatus = components["schemas"]["VaultStatus"];

/** available 为 false 表示服务端还没有隐藏内容的接口（404 或 501）。 */
export type VaultState = VaultStatus & { available: boolean };

export const vaultKeys = {
  status: ["vault", "status"] as const,
};

const LOCKED: VaultState = {
  available: false,
  configured: false,
  unlocked: false,
};

export function isNotLive(error: unknown) {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  );
}

export function useVaultStatus() {
  return useQuery({
    queryKey: vaultKeys.status,
    queryFn: async (): Promise<VaultState> => {
      try {
        const data = await unwrap(vaultApi.GET("/vault/status"));
        return { ...data, available: true };
      } catch (error) {
        if (isNotLive(error)) return LOCKED;
        throw error;
      }
    },
    staleTime: 60_000,
  });
}

/** 已解锁时返回 true。其他模块用它决定要不要显示“隐藏”分类。 */
export function useVaultUnlocked() {
  return useVaultStatus().data?.unlocked ?? false;
}

/*
 * 解锁后别的模块的缓存要重新拉，才能出现“隐藏”分类里的内容。
 * 锁定后把所有缓存重置，保证隐藏内容不会留在页面上。
 */
function useAfterChange() {
  const qc = useQueryClient();
  return {
    unlocked: (status: VaultStatus) => {
      markVaultSession(true);
      qc.setQueryData(vaultKeys.status, { ...status, available: true });
      qc.invalidateQueries({
        predicate: (q) => q.queryKey[0] !== "vault",
      });
    },
    locked: () => {
      markVaultSession(false);
      qc.setQueryData<VaultState>(vaultKeys.status, (old) =>
        old ? { ...old, unlocked: false, unlockedUntil: undefined } : old,
      );
      qc.resetQueries({ predicate: (q) => q.queryKey[0] !== "vault" });
    },
  };
}

export function useSetupVault() {
  const after = useAfterChange();
  return useMutation({
    mutationFn: (password: string) =>
      unwrap(vaultApi.POST("/vault/setup", { body: { password } })),
    onSuccess: after.unlocked,
  });
}

export function useUnlockVault() {
  const after = useAfterChange();
  return useMutation({
    mutationFn: (password: string) =>
      unwrap(vaultApi.POST("/vault/unlock", { body: { password } })),
    onSuccess: after.unlocked,
  });
}

export function useLockVault() {
  const after = useAfterChange();
  return useMutation({
    mutationFn: () => unwrap(vaultApi.POST("/vault/lock")),
    // 请求失败也在页面上锁住。服务端 15 分钟后自己也会锁。
    onSettled: after.locked,
  });
}

export function useChangeVaultPassword() {
  return useMutation({
    mutationFn: (body: { oldPassword: string; newPassword: string }) =>
      unwrap(vaultApi.POST("/vault/password", { body })),
  });
}
