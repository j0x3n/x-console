import { create } from "zustand";

/*
 * PWA（B5）：注册 service worker，记下浏览器的安装事件。
 * beforeinstallprompt 可能在页面刚打开时就触发，所以在模块加载时就监听。
 */
export interface InstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

interface InstallState {
  prompt: InstallPromptEvent | null;
  installed: boolean;
  /** 个人菜单里点“安装应用”但浏览器不能直接装时，弹出说明。 */
  helpOpen: boolean;
  install: () => Promise<boolean>;
}

export function isStandalone(): boolean {
  if (typeof window === "undefined") return false;
  return (
    window.matchMedia?.("(display-mode: standalone)").matches ||
    window.matchMedia?.("(display-mode: fullscreen)").matches ||
    window.matchMedia?.("(display-mode: minimal-ui)").matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  );
}

/** iPhone 和 iPad 上的 Safari 没有安装事件，只能手动“添加到主屏幕”。 */
export function isIOS(ua = navigator.userAgent): boolean {
  return (
    /iPad|iPhone|iPod/.test(ua) ||
    (/Macintosh/.test(ua) &&
      typeof navigator !== "undefined" &&
      navigator.maxTouchPoints > 1)
  );
}

export const useInstall = create<InstallState>()((set, get) => ({
  prompt: null,
  installed: isStandalone(),
  helpOpen: false,
  install: async () => {
    const event = get().prompt;
    if (!event) return false;
    await event.prompt();
    const { outcome } = await event.userChoice;
    set({ prompt: null });
    return outcome === "accepted";
  },
}));

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (e) => {
    e.preventDefault();
    useInstall.setState({ prompt: e as InstallPromptEvent });
  });
  window.addEventListener("appinstalled", () =>
    useInstall.setState({ installed: true, prompt: null }),
  );
}

/** 生产环境里注册 service worker。推送通知用的是同一个。 */
export function registerServiceWorker() {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) return;
  navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch(() => {
    /* 注册失败只是没有离线缓存，不影响使用 */
  });
}
