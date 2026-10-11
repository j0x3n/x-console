import { useEffect, useState } from "react";
import Dialog from "../../components/ui/Dialog";
import { Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../lib/time";
import {
  useClearManual,
  useMeHistory,
  useSetManual,
  type MeDuration,
  type MeState,
  type MeStateName,
} from "./api";
import {
  DURATIONS,
  MANUAL_STATES,
  STATE_LABELS,
  manualBadge,
  sleepLine,
} from "./logic";
import "./i18n";
import "./me.css";

/** 点开“我的状态”卡片：手动设定和最近 7 天的睡觉、起床时间。 */
export default function MeDialog({
  open,
  state,
  onClose,
  onError,
}: {
  open: boolean;
  state: MeState;
  onClose: () => void;
  onError: (error: unknown) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const history = useMeHistory(7, open);
  const set = useSetManual();
  const clear = useClearManual();
  const [pick, setPick] = useState<MeStateName>("home");
  const [duration, setDuration] = useState<MeDuration>("2h");
  // 每次打开时，从现在的状态开始选
  useEffect(() => {
    if (open && state.state !== "unknown") setPick(state.state);
  }, [open, state.state]);

  const time = (d: Date) => formatTime(d, language);
  const date = (d: Date) => formatDate(d, language);
  const apply = () =>
    set.mutate(
      { state: pick, duration },
      { onSuccess: () => toast(t("Saved")), onError },
    );
  const back = () =>
    clear.mutate(undefined, {
      onSuccess: () => toast(t("Back to automatic")),
      onError,
    });

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("My status")}
      wide
      footer={
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      }
    >
      <section className="me-dialog">
        <h3>{t("Set it by hand")}</h3>
        <p className="xc-muted">
          {t(
            "Use this when the guess is wrong, for example when you lie in bed with the sleep mode on.",
          )}
        </p>
        <div className="me-pick" role="group" aria-label={t("Status")}>
          {MANUAL_STATES.map((s) => (
            <button
              key={s}
              type="button"
              className={`xc-btn small${pick === s ? " primary" : ""}`}
              aria-pressed={pick === s}
              onClick={() => setPick(s)}
            >
              {t(STATE_LABELS[s])}
            </button>
          ))}
        </div>
        <div className="me-dialog-row">
          <label className="xc-field">
            <span>{t("For how long")}</span>
            <select
              className="xc-select"
              value={duration}
              onChange={(e) => setDuration(e.target.value as MeDuration)}
            >
              {DURATIONS.map((d) => (
                <option key={d.id} value={d.id}>
                  {t(d.label)}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="xc-btn primary"
            disabled={set.isPending}
            onClick={apply}
          >
            {t("Set status")}
          </button>
        </div>
        {state.manual && (
          <div className="me-dialog-manual">
            <span className="xc-muted">
              {manualBadge(t, state, new Date(), time, date)}
            </span>
            <button
              type="button"
              className="xc-btn small"
              disabled={clear.isPending}
              onClick={back}
            >
              {t("Back to automatic")}
            </button>
          </div>
        )}
      </section>

      <section className="me-dialog">
        <h3>{t("Last 7 days")}</h3>
        {history.isPending ? (
          <Loading />
        ) : history.isError ? (
          <p className="xc-muted">{t("Could not load the history")}</p>
        ) : (
          <ul className="me-history">
            {history.data.days.map((d) => {
              const line = sleepLine(t, d, time);
              return (
                <li key={d.date}>
                  <span>{date(new Date(`${d.date}T12:00:00`))}</span>
                  <span className="me-history-sleep">
                    {line || <span className="xc-muted">—</span>}
                    {line && d.sleepGuessed && (
                      <span className="xc-badge me-today-tag">
                        {t("Guessed")}
                      </span>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
        <small className="xc-muted">
          {t("Only sleeps of 3 hours or more count as a night's sleep.")}
        </small>
      </section>
    </Dialog>
  );
}
