import { create } from "zustand";
import { ApiError } from "../api/client";

/*
 * 高危操作（开终端、执行脚本、吊销代理等）需要 5 分钟内验证过 TOTP。
 * 用法：await withElevation(() => mutateAsync(input))
 * 服务端返回 elevation_required 时弹出验证码框，验证成功后自动重试一次。
 */
interface ElevationState {
  open: boolean;
  resolve?: () => void;
  reject?: (reason: unknown) => void;
  request: () => Promise<void>;
  finish: (ok: boolean) => void;
}

export const useElevationStore = create<ElevationState>()((set, get) => ({
  open: false,
  request: () =>
    new Promise<void>((resolve, reject) => {
      get().reject?.(new Error("replaced"));
      set({ open: true, resolve, reject });
    }),
  finish: (ok) => {
    const { resolve, reject } = get();
    set({ open: false, resolve: undefined, reject: undefined });
    if (ok) resolve?.();
    else reject?.(new ApiError(403, "elevation_canceled", "已取消验证"));
  },
}));

export async function withElevation<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (error) {
    if (error instanceof ApiError && error.code === "elevation_required") {
      await useElevationStore.getState().request();
      return fn();
    }
    throw error;
  }
}
