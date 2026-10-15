import { isStandalone } from "../pwa/install";

/**
 * B164：这个页面算不算“手机上的面板”。屏幕窄于 768px，或者是添加到主屏幕、
 * 用手指操作的网页版。
 */
export function isPhonePanel(): boolean {
  if (typeof window === "undefined") return false;
  if (window.matchMedia?.("(max-width: 767px)").matches) return true;
  return isStandalone() && !!window.matchMedia?.("(pointer: coarse)").matches;
}

/** 页面在前台、是手机面板时，每分钟调一次 fn，切回前台时马上调一次。 */
export function everyMinuteOnPhone(fn: () => void): () => void {
  const tick = () => {
    if (document.visibilityState === "visible" && isPhonePanel()) fn();
  };
  tick();
  const timer = window.setInterval(tick, 60_000);
  document.addEventListener("visibilitychange", tick);
  return () => {
    window.clearInterval(timer);
    document.removeEventListener("visibilitychange", tick);
  };
}
