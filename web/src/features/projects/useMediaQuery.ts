import { useSyncExternalStore } from "react";

/** 媒体查询是否匹配，比如手机宽度。和日历模块的同名函数一样，模块之间不互相引用。 */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (fn) => {
      if (typeof window === "undefined" || !window.matchMedia) return () => {};
      const mq = window.matchMedia(query);
      mq.addEventListener("change", fn);
      return () => mq.removeEventListener("change", fn);
    },
    () =>
      typeof window !== "undefined" &&
      !!window.matchMedia &&
      window.matchMedia(query).matches,
    () => false,
  );
}
