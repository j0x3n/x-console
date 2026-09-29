import { unwrap } from "../../api/client";
import { remindersApi } from "./api";

/*
 * 浏览器推送。只有用户在设置页点“开启”时才注册 service worker（/sw.js），
 * 然后用服务端的 VAPID 公钥订阅，把订阅发给服务端。
 */

export type PushState = "unsupported" | "denied" | "enabled" | "disabled";

export const SW_URL = "/sw.js";

/** VAPID 公钥是 base64url，PushManager 要 Uint8Array。 */
export function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padded = (base64 + "=".repeat((4 - (base64.length % 4)) % 4))
    .replace(/-/g, "+")
    .replace(/_/g, "/");
  const raw = atob(padded);
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

/** 订阅对象转成接口需要的结构（去掉 expirationTime 之类的多余字段）。 */
export function subscriptionBody(sub: PushSubscriptionJSON, userAgent = "") {
  return {
    endpoint: sub.endpoint ?? "",
    keys: { p256dh: sub.keys?.p256dh ?? "", auth: sub.keys?.auth ?? "" },
    userAgent,
  };
}

export function pushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

async function existingSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.getRegistration(SW_URL);
  return (await reg?.pushManager.getSubscription()) ?? null;
}

export async function pushState(): Promise<PushState> {
  if (!pushSupported()) return "unsupported";
  if (Notification.permission === "denied") return "denied";
  return (await existingSubscription()) ? "enabled" : "disabled";
}

export async function enablePush(): Promise<PushState> {
  if (!pushSupported()) return "unsupported";
  const permission = await Notification.requestPermission();
  if (permission !== "granted") return "denied";
  const reg = await navigator.serviceWorker.register(SW_URL, { scope: "/" });
  await navigator.serviceWorker.ready;
  const { publicKey } = await unwrap(
    remindersApi.GET("/notify/webpush/vapid-public-key"),
  );
  let sub = await reg.pushManager.getSubscription();
  if (!sub) {
    sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(publicKey),
    });
  }
  await unwrap(
    remindersApi.POST("/notify/webpush/subscriptions", {
      body: subscriptionBody(sub.toJSON(), navigator.userAgent),
    }),
  );
  return "enabled";
}

export async function disablePush(): Promise<PushState> {
  if (!pushSupported()) return "unsupported";
  const sub = await existingSubscription();
  if (sub) {
    try {
      await unwrap(
        remindersApi.DELETE("/notify/webpush/subscriptions", {
          params: { query: { endpoint: sub.endpoint } },
        }),
      );
    } catch {
      /* 服务端可能已经删掉了 */
    }
    await sub.unsubscribe();
  }
  return "disabled";
}

/* ---- B34：推送自检 ---- */

export type PushService =
  | "google"
  | "microsoft"
  | "mozilla"
  | "apple"
  | "other";

/** 按 endpoint 的域名判断推送服务。 */
export function pushService(endpoint: string): PushService {
  let host = "";
  try {
    host = new URL(endpoint).hostname;
  } catch {
    return "other";
  }
  if (host === "fcm.googleapis.com" || host.endsWith(".googleapis.com"))
    return "google";
  if (host.endsWith(".notify.windows.com")) return "microsoft";
  if (host.endsWith(".mozilla.com") || host.endsWith(".mozaws.net"))
    return "mozilla";
  if (host.endsWith(".push.apple.com")) return "apple";
  return "other";
}

export type Platform =
  | "windows"
  | "macos"
  | "android"
  | "ios"
  | "linux"
  | "other";

export function platformOf(ua: string): Platform {
  if (/iPhone|iPad|iPod/.test(ua)) return "ios";
  if (/Android/.test(ua)) return "android";
  if (/Windows/.test(ua)) return "windows";
  if (/Mac OS X|Macintosh/.test(ua)) return "macos";
  if (/Linux/.test(ua)) return "linux";
  return "other";
}

/** 从 User-Agent 看出浏览器和系统，比如“Edge · Windows”。 */
export function deviceName(ua: string): string {
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /OPR\//.test(ua)
        ? "Opera"
        : /Chrome\//.test(ua)
          ? "Chrome"
          : /Safari\//.test(ua)
            ? "Safari"
            : "";
  const os: Record<Platform, string> = {
    windows: "Windows",
    macos: "macOS",
    android: "Android",
    ios: "iOS",
    linux: "Linux",
    other: "",
  };
  return [browser, os[platformOf(ua)]].filter(Boolean).join(" · ");
}

export async function currentEndpoint(): Promise<string | null> {
  if (!pushSupported()) return null;
  return (await existingSubscription())?.endpoint ?? null;
}

export type LocalTest =
  | "shown"
  | "blocked"
  | "not-asked"
  | "unsupported"
  | "failed";

/**
 * 本机检查：不经过服务端，直接让 service worker 弹一条通知。
 * 返回 shown 只说明浏览器接受了，系统层面关了通知时浏览器也不知道。
 */
export async function localTestNotification(
  title: string,
  body: string,
): Promise<LocalTest> {
  if (!pushSupported()) return "unsupported";
  if (Notification.permission === "denied") return "blocked";
  if (Notification.permission !== "granted") return "not-asked";
  try {
    const reg =
      (await navigator.serviceWorker.getRegistration(SW_URL)) ??
      (await navigator.serviceWorker.register(SW_URL, { scope: "/" }));
    await reg.showNotification(title, {
      body,
      icon: "/icons/icon-192.png",
      tag: "xc-local-test",
    });
    return "shown";
  } catch {
    return "failed";
  }
}
