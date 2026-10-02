/*
 * X Console service worker: Web Push notifications (M7).
 *
 * The server sends a JSON payload: {id, kind, title, body, link, priority,
 * actions: [{action, title}]}. Clicking the notification opens its link.
 * Clicking a button calls POST /api/v1/notify/actions with the action id,
 * for example "reminder.done:42" or "habit.checkin:3:1".
 *
 * PWA caching (B5) is below: the app opens offline with the last shell it
 * saw. API requests are never cached.
 */

// ---- PWA caching (B5) ----
// Bump SHELL_CACHE when the list below changes.
const SHELL_CACHE = "xc-shell-v2";
// v2：旧版本可能把服务端回的 index.html 当成 JS 存了进去，换个名字清掉。
const ASSET_CACHE = "xc-assets-v2";
const SHELL = [
  "/",
  "/manifest.webmanifest",
  "/favicon.svg",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
];
// Built files under /assets/ have a content hash in the name, so a cached
// copy never goes stale. Keep the newest ones only.
const MAX_ASSETS = 80;

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.addAll(SHELL))
      .catch(() => undefined)
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter(
              (k) =>
                (k.startsWith("xc-shell-") && k !== SHELL_CACHE) ||
                (k.startsWith("xc-assets") && k !== ASSET_CACHE),
            )
            .map((k) => caches.delete(k)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

async function trimAssets() {
  const cache = await caches.open(ASSET_CACHE);
  const keys = await cache.keys();
  for (const key of keys.slice(0, Math.max(0, keys.length - MAX_ASSETS)))
    await cache.delete(key);
}

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  // API, WebSocket upgrades and uploads/downloads always go to the server.
  if (url.pathname.startsWith("/api/")) return;

  // Pages: network first so a new deploy shows up at once; offline falls back
  // to the last shell.
  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request)
        .then((response) => {
          if (response.ok) {
            const copy = response.clone();
            caches.open(SHELL_CACHE).then((cache) => cache.put("/", copy));
          }
          return response;
        })
        .catch(() => caches.match("/").then((r) => r || Response.error())),
    );
    return;
  }

  // Hashed build files: cache first.
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(
      caches.match(request).then(
        (hit) =>
          hit ||
          fetch(request).then((response) => {
            // 只存真的文件。部署后旧文件不在了，不能把错误页面存成 JS。
            const html = (response.headers.get("Content-Type") || "").includes(
              "text/html",
            );
            if (response.ok && !html) {
              const copy = response.clone();
              caches
                .open(ASSET_CACHE)
                .then((cache) => cache.put(request, copy))
                .then(trimAssets);
            }
            return response;
          }),
      ),
    );
    return;
  }

  // Icons, manifest and the rest: use the cache, refresh it in the background.
  if (SHELL.includes(url.pathname) || url.pathname.startsWith("/icons/")) {
    event.respondWith(
      caches.open(SHELL_CACHE).then((cache) =>
        cache.match(request).then((hit) => {
          const refresh = fetch(request)
            .then((response) => {
              if (response.ok) cache.put(request, response.clone());
              return response;
            })
            .catch(() => hit || Response.error());
          return hit || refresh;
        }),
      ),
    );
  }
});
// ---- end of PWA caching ----

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { title: event.data ? event.data.text() : "X Console" };
  }
  const title = data.title || "X Console";
  const actions = Array.isArray(data.actions) ? data.actions.slice(0, 2) : [];
  // B52：服务端带上发送时间。晚到 10 分钟以上的，正文后面写上原来的时间。
  const sentAt = data.sentAt ? Date.parse(data.sentAt) : NaN;
  let body = data.body || "";
  if (!Number.isNaN(sentAt) && Date.now() - sentAt > 10 * 60 * 1000) {
    const when = new Date(sentAt).toLocaleString("zh-CN", {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
    body = `${body}${body ? "\n" : ""}（发送于 ${when}）`;
  }
  const options = {
    body,
    timestamp: Number.isNaN(sentAt) ? undefined : sentAt,
    tag: data.id ? `xc-${data.id}` : undefined,
    icon: "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    data: { link: data.link || "/", actions },
    actions: actions.map((a) => ({ action: a.action, title: a.title })),
    requireInteraction: data.priority === "urgent" || actions.length > 0,
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

async function openLink(link) {
  const url = new URL(link || "/", self.location.origin).href;
  const windows = await self.clients.matchAll({
    type: "window",
    includeUncontrolled: true,
  });
  for (const client of windows) {
    if (
      new URL(client.url).origin === self.location.origin &&
      "focus" in client
    ) {
      await client.focus();
      if ("navigate" in client) {
        try {
          await client.navigate(url);
        } catch {
          /* cross-origin or not controlled: focusing is enough */
        }
      }
      return;
    }
  }
  await self.clients.openWindow(url);
}

async function runAction(actionId, link) {
  try {
    const response = await fetch("/api/v1/notify/actions", {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-Requested-With": "x-console",
      },
      body: JSON.stringify({ actionId }),
    });
    if (response.status === 401) {
      // Signed out: open the app so the user can sign in and act there.
      await openLink(link);
      return;
    }
    if (!response.ok) throw new Error(String(response.status));
  } catch {
    await self.registration.showNotification("操作没有完成", {
      body: "请打开 X Console 再试一次。",
      tag: "xc-action-failed",
      data: { link: link || "/" },
    });
  }
}

self.addEventListener("notificationclick", (event) => {
  const { link } = event.notification.data || {};
  event.notification.close();
  if (event.action) {
    event.waitUntil(runAction(event.action, link));
  } else {
    event.waitUntil(openLink(link));
  }
});

// B52：浏览器自己换了推送订阅时，把新订阅交给服务端，删掉旧的。
// 面板打开时 PushSync 也会再同步一次，这里只是尽量早一点接上。
function b64ToBytes(base64) {
  const padded = (base64 + "=".repeat((4 - (base64.length % 4)) % 4))
    .replace(/-/g, "+")
    .replace(/_/g, "/");
  const raw = atob(padded);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

self.addEventListener("pushsubscriptionchange", (event) => {
  const headers = {
    "Content-Type": "application/json",
    "X-Requested-With": "x-console",
  };
  event.waitUntil(
    (async () => {
      let sub = event.newSubscription;
      if (!sub) {
        const res = await fetch("/api/v1/notify/webpush/vapid-public-key", {
          credentials: "same-origin",
          headers,
        });
        if (!res.ok) return;
        const { publicKey } = await res.json();
        sub = await self.registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: b64ToBytes(publicKey),
        });
      }
      const json = sub.toJSON();
      await fetch("/api/v1/notify/webpush/subscriptions", {
        method: "POST",
        credentials: "same-origin",
        headers,
        body: JSON.stringify({
          endpoint: json.endpoint,
          keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
          userAgent: self.navigator.userAgent,
        }),
      });
      const old = event.oldSubscription;
      if (old && old.endpoint !== json.endpoint)
        await fetch(
          "/api/v1/notify/webpush/subscriptions?endpoint=" +
            encodeURIComponent(old.endpoint),
          { method: "DELETE", credentials: "same-origin", headers },
        );
    })().catch(() => {}),
  );
});
