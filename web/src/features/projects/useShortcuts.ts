import { useEffect, useRef } from "react";

/** 焦点在输入框、或者有弹窗打开时，单键快捷键不生效。 */
export function shouldIgnoreKey(event: KeyboardEvent): boolean {
  if (event.metaKey || event.ctrlKey || event.altKey || event.isComposing)
    return true;
  const target = event.target as HTMLElement | null;
  if (
    target &&
    (target.isContentEditable ||
      ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))
  )
    return true;
  return document.querySelector(".modal-backdrop") !== null;
}

/**
 * 单键快捷键。handlers 的键是 event.key 的小写，比如 "c"、"j"、"enter"、"escape"、"1"。
 * 处理函数返回 false 表示没处理，不拦截默认行为。
 */
export function useShortcuts(
  handlers: Record<string, (event: KeyboardEvent) => unknown>,
  enabled = true,
) {
  const ref = useRef(handlers);
  ref.current = handlers;
  useEffect(() => {
    if (!enabled) return;
    const onKey = (event: KeyboardEvent) => {
      const handler = ref.current[event.key.toLowerCase()];
      if (!handler) return;
      // Escape 在输入框里也要能用（先让输入框失焦）。
      if (event.key === "Escape") {
        const target = event.target as HTMLElement | null;
        if (document.querySelector(".modal-backdrop")) return;
        if (
          target &&
          ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
        ) {
          target.blur();
          return;
        }
      } else if (shouldIgnoreKey(event)) return;
      if (handler(event) !== false) event.preventDefault();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [enabled]);
}
