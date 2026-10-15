import { useEffect, useState } from "react";
import { Plus, Send, Trash2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import Switch from "../../components/ui/Switch";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatTime, relativeTime } from "../../lib/time";
import {
  useDigest,
  useFlushDigest,
  useSaveDigest,
  type DigestMode,
} from "./api";
import "./i18n";
import "./reminders.css";

/** B127：每一类通知的名字，id 和后端的分类一致。 */
const CATEGORY_LABELS: Record<string, string> = {
  alert: "Server and monitor alerts",
  security: "Security alerts",
  agent_decision: "Agent waits for you",
  reminder: "Reminders and habits",
  brief: "Brief and weather",
  agent_progress: "Agent and coding progress",
  mail: "New mail alerts",
  github: "GitHub activity",
  project: "Cards and subscriptions due",
  other: "Everything else",
};

const MAX_TIMES = 6;
const CLOCK = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;

/** 时间点都填了、格式对、没有重复才能保存。 */
export function timesError(times: string[]): boolean {
  return (
    times.length === 0 ||
    times.some((v) => !CLOCK.test(v)) ||
    new Set(times).size !== times.length
  );
}

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 设置 → 通知里的“推送合并”卡片。 */
export default function DigestCard() {
  const t = useT();
  const language = useLanguage();
  const digest = useDigest();
  const save = useSaveDigest();
  const flush = useFlushDigest();
  const [enabled, setEnabled] = useState(false);
  const [times, setTimes] = useState<string[]>([]);
  const [modes, setModes] = useState<Record<string, DigestMode>>({});
  const [outDirect, setOutDirect] = useState(true);

  const data = digest.data;
  // 只在服务端的设置真的变了时才覆盖表单，积压数量变化不会冲掉没保存的修改。
  const signature = data
    ? JSON.stringify([
        data.enabled,
        data.times,
        data.categories.map((c) => [c.id, c.mode]),
        data.outDirect,
      ])
    : "";
  useEffect(() => {
    if (!signature) return;
    const [on, ts, cats, out] = JSON.parse(signature) as [
      boolean,
      string[],
      [string, DigestMode][],
      boolean,
    ];
    setEnabled(on);
    setTimes(ts);
    setModes(Object.fromEntries(cats));
    setOutDirect(out);
  }, [signature]);

  if (digest.isPending) return <Loading />;
  if (digest.isError)
    return isNotLive(digest.error) ? (
      <NotLive name="推送合并" />
    ) : (
      <ErrorState error={digest.error} onRetry={() => digest.refetch()} />
    );
  const status = digest.data;

  const dirty =
    JSON.stringify([
      enabled,
      times,
      status.categories.map((c) => modes[c.id]),
      outDirect,
    ]) !==
    JSON.stringify([
      status.enabled,
      status.times,
      status.categories.map((c) => c.mode),
      status.outDirect,
    ]);
  const invalid = timesError(times);

  return (
    <section className="xc-card notify-digest">
      <div className="xc-card-head">
        <h2>{t("Batched push")}</h2>
        <Switch
          checked={enabled}
          label={t("Turn on batched push")}
          onChange={setEnabled}
        />
      </div>
      <p className="xc-muted notify-help">
        {t(
          "Notifications that are not urgent wait, then go out together as one message at the times below. They show up in the app at once.",
        )}
      </p>

      <div className="digest-status">
        <span>
          <strong>{status.pending}</strong> {t("notifications waiting")}
        </span>
        {status.oldestAt && (
          <small className="xc-muted">
            {t("Oldest")} {relativeTime(status.oldestAt, language)}
          </small>
        )}
        {status.pendingWorking > 0 && (
          <small className="xc-muted">
            {t("{n} waiting for the computer").replace(
              "{n}",
              String(status.pendingWorking),
            )}
          </small>
        )}
        {status.enabled && status.nextAt && (
          <small className="xc-muted">
            {t("Next send")} {formatTime(status.nextAt, language)}
          </small>
        )}
        {status.lastSentAt && (
          <small className="xc-muted">
            {t("Last sent")} {relativeTime(status.lastSentAt, language)}
          </small>
        )}
        <span className="xc-spacer" />
        <button
          className="xc-btn small"
          disabled={status.pending === 0 || flush.isPending}
          onClick={() =>
            flush.mutate(undefined, {
              onSuccess: (res) =>
                toast(
                  res.count === 0
                    ? t("Nothing is waiting")
                    : `${t("Batch sent")} · ${res.count}`,
                ),
              onError,
            })
          }
        >
          <Send size={13} /> {t("Send waiting now")}
        </button>
      </div>

      <div className="xc-field">
        <span>{t("Send times")}</span>
        <div className="digest-times">
          {times.map((v, i) => (
            <span className="digest-time" key={i}>
              <input
                className="xc-input"
                type="time"
                aria-label={`${t("Send time")} ${i + 1}`}
                value={v}
                onChange={(e) =>
                  setTimes(times.map((x, j) => (j === i ? e.target.value : x)))
                }
              />
              {times.length > 1 && (
                <button
                  className="xc-btn ghost small"
                  aria-label={t("Delete")}
                  onClick={() => setTimes(times.filter((_, j) => j !== i))}
                >
                  <Trash2 size={13} />
                </button>
              )}
            </span>
          ))}
          {times.length < MAX_TIMES && (
            <button
              className="xc-btn small"
              onClick={() => setTimes([...times, ""])}
            >
              <Plus size={13} /> {t("Add time")}
            </button>
          )}
        </div>
        <small>
          {t(
            "Uses the server time zone. In quiet hours the batch is sent after they end.",
          )}
        </small>
      </div>

      <div className="xc-field">
        <span>{t("Each type")}</span>
        <small>
          {t(
            "Urgent notifications and notifications with buttons are always sent at once.",
          )}{" "}
          {t(
            "When working: sent at once while you use a work computer. Otherwise they wait until you are back at it, or show on the phone when the panel is open there. This works with batched push off too.",
          )}
        </small>
        <ul className="digest-types">
          {status.categories.map((c) => {
            const label = t(CATEGORY_LABELS[c.id] ?? c.id);
            return (
              <li key={c.id}>
                <span>{label}</span>
                <select
                  className="xc-select"
                  aria-label={label}
                  value={modes[c.id] ?? c.mode}
                  onChange={(e) =>
                    setModes({ ...modes, [c.id]: e.target.value as DigestMode })
                  }
                >
                  <option value="now">{t("Immediately")}</option>
                  <option value="digest">{t("In batch")}</option>
                  <option value="working">{t("When working")}</option>
                </select>
              </li>
            );
          })}
        </ul>
      </div>

      <label className="reminders-check">
        <input
          type="checkbox"
          checked={outDirect}
          onChange={(e) => setOutDirect(e.target.checked)}
        />
        {t("When I am out, push “when working” ones to the phone at once")}
      </label>

      <div className="xc-row">
        <span className="xc-spacer" />
        <button
          className="xc-btn small primary"
          disabled={!dirty || invalid || save.isPending}
          onClick={() =>
            save.mutate(
              {
                enabled,
                times,
                modes: Object.fromEntries(
                  status.categories.map((c) => [c.id, modes[c.id] ?? c.mode]),
                ),
                outDirect,
              },
              { onSuccess: () => toast(t("Saved")), onError },
            )
          }
        >
          {t("Save")}
        </button>
      </div>
    </section>
  );
}
