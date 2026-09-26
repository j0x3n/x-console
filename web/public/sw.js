/*
 * X Console service worker: Web Push notifications (M7).
 *
 * The server sends a JSON payload: {id, kind, title, body, link, priority,
 * actions: [{action, title}]}. Clicking the notification opens its link.
 * Clicking a button calls POST /api/v1/notify/actions with the action id,
 * for example "reminder.done:42" or "habit.checkin:3:1".
 *
 * Task card J (PWA) merges its caching logic into this file.
 */

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { title: event.data ? event.data.text() : "X Console" };
  }
  const title = data.title || "X Console";
  const actions = Array.isArray(data.actions) ? data.actions.slice(0, 2) : [];
  const options = {
    body: data.body || "",
    tag: data.id ? `xc-${data.id}` : undefined,
    icon: "/favicon.svg",
    badge: "/favicon.svg",
    data: { link: data.link || "/", actions },
    actions: actions.map((a) => ({ action: a.action, title: a.title })),
    requireInteraction: data.priority === "urgent" || actions.length > 0,
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

async function openLink(link) {
  const url = new URL(link || "/", self.location.origin).href;
  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  for (const client of windows) {
    if (new URL(client.url).origin === self.location.origin && "focus" in client) {
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
      headers: { "Content-Type": "application/json", "X-Requested-With": "x-console" },
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
