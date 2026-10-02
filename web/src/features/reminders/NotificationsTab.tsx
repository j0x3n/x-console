import { useEffect, useState } from "react";
import { Link } from "react-router";
import {
  ArrowDown,
  ArrowUp,
  BellRing,
  Plus,
  Send,
  Trash2,
  Webhook,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { toast } from "../../hooks/useToast";
import {
  useDeletePushSubscription,
  useNotifyChannels,
  useNotifyRoutes,
  usePushSubscriptions,
  useQuietHours,
  useRegisterTelegramWebhook,
  useSaveQuietHours,
  useSaveRoutes,
  useTestChannel,
  useTestWebPush,
  useUpdateChannel,
  type ChannelName,
  type NotifyChannel,
  type NotifyPriority,
  type NotifyRoute,
  type WebPushSubscriptionInfo,
  type WebPushTestResult,
} from "./api";
import "./i18n";
import "./reminders.css";
import {
  currentEndpoint,
  deviceName,
  disablePush,
  enablePush,
  localTestNotification,
  platformOf,
  pushService,
  pushState,
  type LocalTest,
  type Platform,
  type PushService,
  type PushState,
} from "./push";

export const channelLabels: Record<ChannelName, string> = {
  webpush: "Browser push",
  telegram: "Telegram",
  bark: "Bark",
  serverchan: "ServerChan",
};

const fieldLabels: Record<string, string> = {
  bot_token: "Bot token",
  chat_id: "Chat ID",
  api_base: "API address",
  device_key: "Device key",
  server: "Server address",
  send_key: "SendKey",
  subject: "Contact email",
};

const channelHelp: Record<ChannelName, string> = {
  webpush: "Push to this browser. Each browser is enabled on its own.",
  telegram:
    "Create a bot with @BotFather. Send it a message, then use your chat id.",
  bark: "Push to iPhone with the Bark app.",
  serverchan: "Push to WeChat through ServerChan.",
};

const priorities: NotifyPriority[] = ["low", "normal", "high", "urgent"];
const priorityLabels: Record<NotifyPriority, string> = {
  low: "Low",
  normal: "Normal",
  high: "High",
  urgent: "Urgent",
};

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

export default function NotificationsTab() {
  const channels = useNotifyChannels();
  if (channels.isPending) return <Loading />;
  if (channels.isError)
    return (
      <ErrorState error={channels.error} onRetry={() => channels.refetch()} />
    );
  return (
    <div className="xc-stack notify-settings">
      <div className="xc-grid notify-channels">
        {channels.data.map((ch) => (
          <ChannelCard key={ch.name} channel={ch} />
        ))}
      </div>
      <RoutesCard channels={channels.data.map((c) => c.name as ChannelName)} />
      <QuietHoursCard />
      <ScheduleLinkCard />
    </div>
  );
}

/** B83：作息在习惯页里设，这里给一个入口。 */
function ScheduleLinkCard() {
  const t = useT();
  return (
    <section className="xc-card notify-schedule-link">
      <div className="xc-card-head">
        <h2>{t("Daily schedule")}</h2>
        <Link className="xc-btn small" to="/habits?schedule=1">
          {t("Set daily schedule")}
        </Link>
      </div>
      <p className="xc-muted">
        {t("Health reminders use your work days, wake and sleep times.")}
      </p>
    </section>
  );
}

function ChannelCard({ channel }: { channel: NotifyChannel }) {
  const t = useT();
  const name = channel.name as ChannelName;
  const update = useUpdateChannel();
  const test = useTestChannel();
  const [values, setValues] = useState<Record<string, string>>({});

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const f of channel.fields) next[f.key] = f.secret ? "" : f.value;
    setValues(next);
  }, [channel]);

  const changed: Record<string, string> = {};
  for (const f of channel.fields) {
    const v = values[f.key] ?? "";
    if (f.secret ? v !== "" : v !== f.value) changed[f.key] = v;
  }
  const dirty = Object.keys(changed).length > 0;

  const save = () =>
    withElevation(() =>
      update.mutateAsync({ channel: name, values: changed }),
    ).then(() => toast(t("Saved")), onError);

  return (
    <div className="xc-card notify-channel">
      <div className="xc-card-head">
        <h2>{t(channelLabels[name])}</h2>
        <span className={`xc-badge ${channel.configured ? "ok" : ""}`}>
          {channel.configured ? t("Ready") : t("Not set up")}
        </span>
      </div>
      <p className="xc-muted notify-help">{t(channelHelp[name])}</p>
      {name === "webpush" && <PushControls channel={channel} />}
      {channel.fields.map((f) => (
        <label className="xc-field" key={f.key}>
          <span>
            {t(fieldLabels[f.key] ?? f.key)}
            {!f.required && (
              <small className="xc-muted"> · {t("optional")}</small>
            )}
          </span>
          <input
            className="xc-input"
            type={f.secret ? "password" : "text"}
            autoComplete="off"
            value={values[f.key] ?? ""}
            placeholder={
              f.secret && f.set
                ? `${t("Saved")} ${f.value}`
                : (f.placeholder ?? "")
            }
            onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
          />
        </label>
      ))}
      {name === "telegram" && <TelegramWebhook channel={channel} />}
      <div className="xc-row notify-channel-actions">
        {name !== "webpush" && (
          <button
            className="xc-btn small"
            disabled={!channel.configured || test.isPending}
            onClick={() =>
              test.mutate(name, {
                onSuccess: () => toast(t("Test message sent")),
                onError,
              })
            }
          >
            <Send size={13} /> {t("Send test")}
          </button>
        )}
        <span className="xc-spacer" />
        <button
          className="xc-btn small primary"
          disabled={!dirty || update.isPending}
          onClick={save}
        >
          {t("Save")}
        </button>
      </div>
    </div>
  );
}

const serviceLabels: Record<PushService, string> = {
  google: "Google (Chrome)",
  microsoft: "Microsoft (Edge)",
  mozilla: "Mozilla (Firefox)",
  apple: "Apple (Safari)",
  other: "Other push service",
};

/** 系统里打开浏览器通知的方法，按平台写一句。 */
const platformHints: Record<Platform, string> = {
  windows:
    "Windows: open Settings, System, Notifications, turn on this browser, and turn off Do not disturb.",
  macos:
    "macOS: open System Settings, Notifications, find this browser and allow notifications.",
  android:
    "Android: open Settings, Apps, this browser, Notifications, and turn them all on.",
  ios: "iPhone: add the panel to the Home Screen, open it from there, then turn on push. Then allow it in Settings, Notifications.",
  linux: "Allow notifications for this browser in your system settings.",
  other: "Allow notifications for this browser in your system settings.",
};

const localMessages: Record<LocalTest, string> = {
  shown:
    "A local notification was sent. If you did not see it, the system is not letting this browser show notifications.",
  blocked: "The browser blocked notifications. Allow them in site settings.",
  "not-asked": "Turn on browser push first.",
  unsupported: "This browser does not support push",
  failed: "The browser could not show a notification.",
};

function PushControls({ channel }: { channel: NotifyChannel }) {
  const t = useT();
  const [state, setState] = useState<PushState | "loading">("loading");
  const [busy, setBusy] = useState(false);
  const [endpoint, setEndpoint] = useState<string | null>(null);
  const [local, setLocal] = useState<LocalTest | null>(null);
  const [server, setServer] = useState<
    WebPushTestResult[] | null | "sent" | { error: string }
  >(null);
  const subs = usePushSubscriptions();
  const testPush = useTestWebPush();
  const platform = platformOf(navigator.userAgent);
  useEffect(() => {
    pushState().then(setState, () => setState("unsupported"));
    currentEndpoint().then(setEndpoint, () => setEndpoint(null));
  }, []);
  const run = async (fn: () => Promise<PushState>) => {
    setBusy(true);
    try {
      const next = await fn();
      setState(next);
      setEndpoint(await currentEndpoint());
      if (next === "denied")
        toast({
          message: t(
            "The browser blocked notifications. Allow them in site settings.",
          ),
          tone: "error",
        });
    } catch (err) {
      onError(err);
    } finally {
      setBusy(false);
    }
  };
  // 两步：先在本机弹一条，再让服务端推一条。
  const runTest = async () => {
    setServer(null);
    setLocal(
      await localTestNotification(
        t("X Console test"),
        t("This one comes from the page itself."),
      ),
    );
    testPush.mutate(undefined, {
      onSuccess: (results) => setServer(results ?? "sent"),
      onError: (err) => setServer({ error: errorMessage(err) }),
    });
  };
  const live = !!subs.data;
  const list = subs.data ?? [];
  const hasGoogle =
    list.some((s) => s.service === "google") ||
    (!!endpoint && pushService(endpoint) === "google");

  return (
    <div className="notify-push">
      <div className="xc-row">
        <BellRing size={15} />
        <span>
          {state === "enabled"
            ? t("This browser gets push notifications")
            : state === "denied"
              ? t("Notifications are blocked in this browser")
              : state === "unsupported"
                ? t("This browser does not support push")
                : t("This browser is not subscribed")}
        </span>
        <span className="xc-spacer" />
        {state === "enabled" ? (
          <button
            className="xc-btn small"
            disabled={busy}
            onClick={() => run(disablePush)}
          >
            {t("Turn off")}
          </button>
        ) : (
          <button
            className="xc-btn small primary"
            disabled={busy || state === "unsupported" || state === "loading"}
            onClick={() => run(enablePush)}
          >
            {t("Enable browser push")}
          </button>
        )}
      </div>
      {live ? (
        <PushSubscriptionList list={list} current={endpoint} />
      ) : (
        <span className="xc-muted">
          {t("Subscribed browsers")}: {channel.subscriptions ?? 0}
        </span>
      )}
      {hasGoogle && (
        <p className="notify-push-warn">
          {t(
            "Chrome pushes through Google. It may not arrive on networks in mainland China. Try Edge or Safari.",
          )}
        </p>
      )}
      <div className="xc-row">
        <button
          className="xc-btn small"
          disabled={!channel.configured || testPush.isPending}
          onClick={() => void runTest()}
        >
          <Send size={13} /> {t("Send test")}
        </button>
      </div>
      {local && (
        <ol className="notify-push-steps">
          <li>
            <strong>{t("Check on this device")}</strong>
            <span>{t(localMessages[local])}</span>
            {(local === "shown" || local === "blocked") && (
              <small>{t(platformHints[platform])}</small>
            )}
          </li>
          <li>
            <strong>{t("Push from the server")}</strong>
            {server === null ? (
              <span className="xc-muted">{t("Sending")}…</span>
            ) : server === "sent" ? (
              <span>{t("Test message sent")}</span>
            ) : "error" in server ? (
              <span className="xc-error-text">{server.error}</span>
            ) : (
              <ul>
                {server.map((r) => {
                  const sub = list.find((s) => s.id === r.id);
                  return (
                    <li key={r.id} className={r.ok ? "ok" : "bad"}>
                      {sub
                        ? deviceName(sub.userAgent) || t("Browser")
                        : `#${r.id}`}
                      {" · "}
                      {t(serviceLabels[r.service])}
                      {" · "}
                      {r.ok
                        ? t("Accepted by the push service")
                        : `${r.status ?? ""} ${r.error ?? ""}`.trim()}
                    </li>
                  );
                })}
              </ul>
            )}
          </li>
        </ol>
      )}
      <details className="notify-push-help">
        <summary>{t("Can I get pushes when the panel is closed?")}</summary>
        <ul>
          <li>
            {t(
              "Yes, as long as the browser is still running. On a computer, keep the browser running in the background.",
            )}
          </li>
          <li>
            {t(
              "iPhone needs iOS 16.4 or later. Add the panel to the Home Screen and turn on push from there.",
            )}
          </li>
          <li>{t("Chrome on Android needs Google services.")}</li>
          <li>
            {t(
              "If a device is offline, messages wait up to 3 days. Messages that arrive late show the time they were sent.",
            )}
          </li>
          <li>
            {t(
              "Push stops on iPhone when the panel is removed from the Home Screen, or when Safari website data is cleared. Turn push on again after that.",
            )}
          </li>
          <li>
            {t("For more reliable alerts, also turn on Bark or Telegram.")}
          </li>
        </ul>
      </details>
    </div>
  );
}

function PushSubscriptionList({
  list,
  current,
}: {
  list: WebPushSubscriptionInfo[];
  current: string | null;
}) {
  const t = useT();
  const language = useLanguage();
  const remove = useDeletePushSubscription();
  if (list.length === 0)
    return (
      <span className="xc-muted">{t("No browser is subscribed yet.")}</span>
    );
  return (
    <ul className="notify-subs">
      {list.map((s) => {
        const name = deviceName(s.userAgent) || t("Browser");
        return (
          <li key={s.id}>
            <div>
              <strong>
                {name}
                {s.endpoint === current && (
                  <span className="xc-badge">{t("This browser")}</span>
                )}
              </strong>
              <small className="xc-muted">
                {t(serviceLabels[s.service])}
                {s.lastOkAt &&
                  ` · ${t("Last delivered")} ${relativeTime(s.lastOkAt, language)}`}
              </small>
              {s.lastError && (
                <small className="xc-error-text">{s.lastError}</small>
              )}
            </div>
            <button
              className="xc-btn small ghost"
              aria-label={`${t("Remove")} ${name}`}
              disabled={remove.isPending}
              onClick={async () =>
                (await confirmAction({
                  title: `${t("Remove")} ${name}？`,
                  description: t(
                    "That browser stops getting push notifications.",
                  ),
                  confirmLabel: t("Remove"),
                })) && remove.mutate(s.id, { onError })
              }
            >
              <Trash2 size={13} />
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function TelegramWebhook({ channel }: { channel: NotifyChannel }) {
  const t = useT();
  const register = useRegisterTelegramWebhook();
  const tokenSet = channel.fields.some((f) => f.key === "bot_token" && f.set);
  return (
    <div className="notify-webhook">
      <p className="xc-muted">
        {channel.webhookUrl
          ? t(
              "Buttons in Telegram need the webhook. Register it once after saving the token.",
            )
          : t("Set XC_PUBLIC_URL on the server to use Telegram buttons.")}
      </p>
      {channel.webhookUrl && (
        <div className="xc-row">
          <code className="xc-mono notify-url">{channel.webhookUrl}</code>
          <button
            className="xc-btn small"
            disabled={!tokenSet || register.isPending}
            onClick={() =>
              register.mutate(undefined, {
                onSuccess: () => toast(t("Webhook registered")),
                onError,
              })
            }
          >
            <Webhook size={13} /> {t("Register webhook")}
          </button>
        </div>
      )}
    </div>
  );
}

function RoutesCard({ channels }: { channels: ChannelName[] }) {
  const t = useT();
  const routes = useNotifyRoutes();
  const save = useSaveRoutes();
  const [rows, setRows] = useState<NotifyRoute[]>([]);
  useEffect(() => {
    if (routes.data) setRows(routes.data);
  }, [routes.data]);

  const set = (i: number, patch: Partial<NotifyRoute>) =>
    setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const move = (i: number, d: number) => {
    const next = [...rows];
    const [row] = next.splice(i, 1);
    next.splice(i + d, 0, row);
    setRows(next);
  };

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Routing rules")}</h2>
        <button
          className="xc-btn small"
          onClick={() =>
            setRows([
              ...rows,
              {
                kindPattern: "*",
                minPriority: "normal",
                channels: [],
                enabled: true,
              },
            ])
          }
        >
          <Plus size={13} /> {t("Add rule")}
        </button>
      </div>
      <p className="xc-muted notify-help">
        {t(
          "Rules are checked from top to bottom. The first match decides the channels. Without a match the notification stays in the app. * matches anything, for example host.alert*.",
        )}
      </p>
      {routes.isPending ? (
        <Loading />
      ) : routes.isError ? (
        <ErrorState error={routes.error} onRetry={() => routes.refetch()} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table notify-routes">
            <thead>
              <tr>
                <th>{t("Kind")}</th>
                <th>{t("At least")}</th>
                <th>{t("Channels")}</th>
                <th>{t("Enabled")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  <td>
                    <input
                      className="xc-input xc-mono"
                      value={r.kindPattern}
                      onChange={(e) => set(i, { kindPattern: e.target.value })}
                    />
                  </td>
                  <td>
                    <select
                      className="xc-select"
                      value={r.minPriority}
                      onChange={(e) =>
                        set(i, {
                          minPriority: e.target.value as NotifyPriority,
                        })
                      }
                    >
                      {priorities.map((p) => (
                        <option key={p} value={p}>
                          {t(priorityLabels[p])}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <div className="notify-route-channels">
                      {channels.map((c) => (
                        <label key={c}>
                          <input
                            type="checkbox"
                            checked={r.channels.includes(c)}
                            onChange={(e) =>
                              set(i, {
                                channels: e.target.checked
                                  ? [...r.channels, c]
                                  : r.channels.filter((x) => x !== c),
                              })
                            }
                          />
                          {t(channelLabels[c])}
                        </label>
                      ))}
                    </div>
                  </td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t("Enabled")}
                      checked={r.enabled}
                      onChange={(e) => set(i, { enabled: e.target.checked })}
                    />
                  </td>
                  <td>
                    <div className="xc-row notify-route-actions">
                      <button
                        className="xc-btn ghost small"
                        aria-label={t("Move up")}
                        disabled={i === 0}
                        onClick={() => move(i, -1)}
                      >
                        <ArrowUp size={13} />
                      </button>
                      <button
                        className="xc-btn ghost small"
                        aria-label={t("Move down")}
                        disabled={i === rows.length - 1}
                        onClick={() => move(i, 1)}
                      >
                        <ArrowDown size={13} />
                      </button>
                      <button
                        className="xc-btn ghost small"
                        aria-label={t("Delete")}
                        onClick={() => setRows(rows.filter((_, j) => j !== i))}
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="xc-dialog-actions">
        <button
          className="xc-btn small primary"
          disabled={save.isPending || !routes.data}
          onClick={() =>
            save.mutate(
              rows.map(({ id: _id, ...r }) => r),
              { onSuccess: () => toast(t("Saved")), onError },
            )
          }
        >
          {t("Save rules")}
        </button>
      </div>
    </div>
  );
}

function QuietHoursCard() {
  const t = useT();
  const quiet = useQuietHours();
  const save = useSaveQuietHours();
  const [enabled, setEnabled] = useState(false);
  const [start, setStart] = useState("23:00");
  const [end, setEnd] = useState("07:30");
  useEffect(() => {
    if (!quiet.data) return;
    setEnabled(quiet.data.enabled);
    setStart(quiet.data.start);
    setEnd(quiet.data.end);
  }, [quiet.data]);
  return (
    <div className="xc-card notify-quiet">
      <div className="xc-card-head">
        <h2>{t("Quiet hours")}</h2>
      </div>
      <p className="xc-muted notify-help">
        {t(
          "During quiet hours only urgent notifications go to your phone. Others stay in the app.",
        )}
      </p>
      <label className="reminders-check">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        {t("Turn on quiet hours")}
      </label>
      <div className="reminders-form-row">
        <label className="xc-field">
          <span>{t("From")}</span>
          <input
            className="xc-input"
            type="time"
            value={start}
            onChange={(e) => setStart(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("To")}</span>
          <input
            className="xc-input"
            type="time"
            value={end}
            onChange={(e) => setEnd(e.target.value)}
          />
        </label>
      </div>
      <div className="xc-row">
        <span className="xc-spacer" />
        <button
          className="xc-btn small primary"
          disabled={save.isPending || !quiet.data}
          onClick={() =>
            save.mutate(
              { enabled, start, end },
              { onSuccess: () => toast(t("Saved")), onError },
            )
          }
        >
          {t("Save")}
        </button>
      </div>
    </div>
  );
}
