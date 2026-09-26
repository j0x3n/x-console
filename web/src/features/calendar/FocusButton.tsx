import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { BarChart3, Check, Play, Square, Timer } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCurrentFocus,
  useStartFocus,
  useStopFocus,
  type FocusSession,
} from "./api";
import { formatRemaining } from "./dates";
import { issuePath } from "../projects/logic";
import { useFocusPanel, useNow } from "./hooks";
import "./i18n";
import "./calendar.css";

const presets = [15, 25, 50];
const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 页头的番茄钟按钮。进行中时显示剩余时间，点开可以开始或结束。 */
export default function FocusButton() {
  const t = useT();
  const current = useCurrentFocus();
  const session = current.data ?? null;
  const open = useFocusPanel((s) => s.open);
  const setOpen = useFocusPanel((s) => s.setOpen);
  const now = useNow(1000, !!session);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) =>
      event.key === "Escape" && setOpen(false);
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, setOpen]);

  const remaining = session
    ? new Date(session.endsAt).getTime() - now.getTime()
    : 0;
  const label = session
    ? remaining > 0
      ? `${t("Focus")} ${formatRemaining(remaining)}`
      : t("Focus time is up")
    : t("Focus");

  return (
    <div className="focus-wrap" ref={ref}>
      <button
        className={`icon-button focus-button${session ? " is-running" : ""}${session && remaining <= 0 ? " is-done" : ""}`}
        aria-label={label}
        title={label}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <Timer size={16} />
        {session && (
          <span className="focus-remaining">
            {remaining > 0 ? formatRemaining(remaining) : "00:00"}
          </span>
        )}
      </button>
      {open && (
        <div className="focus-panel" role="dialog" aria-label={t("Focus")}>
          {session ? (
            <Running session={session} remaining={remaining} />
          ) : (
            <StartForm />
          )}
          <Link
            className="focus-stats-link"
            to="/calendar/focus"
            onClick={() => setOpen(false)}
          >
            <BarChart3 size={13} /> {t("Focus stats")}
          </Link>
        </div>
      )}
    </div>
  );
}

function Running({
  session,
  remaining,
}: {
  session: FocusSession;
  remaining: number;
}) {
  const t = useT();
  const stop = useStopFocus();
  const start = useStartFocus();
  const total = session.plannedMinutes * 60_000;
  const progress = Math.min(1, Math.max(0, 1 - remaining / total));
  const over = remaining <= 0;
  return (
    <div className="xc-stack focus-running">
      <div className="focus-time">{formatRemaining(remaining)}</div>
      <div className="focus-bar">
        <i style={{ width: `${progress * 100}%` }} />
      </div>
      <p className="xc-muted">
        {over
          ? t("Time is up. Take a short break.")
          : `${session.plannedMinutes} ${t("min focus")}`}
      </p>
      {session.issueKey && (
        <Link className="focus-issue" to={issuePath(session.issueKey)}>
          <span className="xc-mono">{session.issueKey}</span>{" "}
          {session.issueTitle}
        </Link>
      )}
      <div className="xc-row">
        {over ? (
          <>
            <button
              className="xc-btn small primary"
              disabled={stop.isPending}
              onClick={() =>
                stop.mutate({ id: session.id, completed: true }, { onError })
              }
            >
              <Check size={14} /> {t("Finish")}
            </button>
            <button
              className="xc-btn small"
              disabled={start.isPending}
              onClick={() =>
                start.mutate(
                  {
                    minutes: session.plannedMinutes,
                    issueKey: session.issueKey || undefined,
                  },
                  { onError },
                )
              }
            >
              <Play size={14} /> {t("One more")}
            </button>
          </>
        ) : (
          <>
            <button
              className="xc-btn small"
              disabled={stop.isPending}
              onClick={() =>
                stop.mutate({ id: session.id, completed: true }, { onError })
              }
            >
              <Check size={14} /> {t("Finish early")}
            </button>
            <button
              className="xc-btn small ghost"
              disabled={stop.isPending}
              onClick={() =>
                stop.mutate({ id: session.id, completed: false }, { onError })
              }
            >
              <Square size={13} /> {t("Give up")}
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function StartForm() {
  const t = useT();
  const start = useStartFocus();
  const presetIssue = useFocusPanel((s) => s.issueKey);
  const [minutes, setMinutes] = useState(25);
  const [issueKey, setIssueKey] = useState(presetIssue);
  useEffect(() => setIssueKey(presetIssue), [presetIssue]);
  const submit = () =>
    start.mutate(
      { minutes, issueKey: issueKey.trim() || undefined },
      {
        onSuccess: () =>
          toast(`${t("Focus started")} · ${minutes} ${t("min")}`),
        onError,
      },
    );
  return (
    <form
      className="xc-stack focus-start"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <div className="focus-presets" role="group" aria-label={t("Minutes")}>
        {presets.map((m) => (
          <button
            type="button"
            key={m}
            className={m === minutes ? "active" : ""}
            aria-pressed={m === minutes}
            onClick={() => setMinutes(m)}
          >
            {m} {t("min")}
          </button>
        ))}
        <input
          className="xc-input"
          type="number"
          min={1}
          max={180}
          value={minutes}
          aria-label={t("Minutes")}
          onChange={(e) =>
            setMinutes(Math.min(180, Math.max(1, Number(e.target.value) || 1)))
          }
        />
      </div>
      <label className="xc-field">
        <span>
          {t("Linked issue")}{" "}
          <small className="xc-muted">{t("optional")}</small>
        </span>
        <input
          className="xc-input"
          value={issueKey}
          placeholder="XC-12"
          onChange={(e) => setIssueKey(e.target.value)}
        />
      </label>
      <button className="xc-btn primary" disabled={start.isPending}>
        <Play size={14} /> {t("Start focus")}
      </button>
    </form>
  );
}
