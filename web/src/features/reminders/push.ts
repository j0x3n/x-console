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
