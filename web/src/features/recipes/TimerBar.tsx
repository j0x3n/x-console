import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";
import { AlarmClock, ChevronDown } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { useCookTimers } from "./api";
import { isDue, playAlarm, remaining, sortTimers, useNow } from "./alarm";
import { clock } from "./durations";
import TimerList from "./TimerList";
import "./i18n";
import "./recipes.css";

/** 已经响过的计时器，同一个只响一次。 */
const rung = new Set<number>();

/*
 * 全局的做菜计时浮条（B209）。有没关掉的计时器时显示在右下角，
 * 前端自己每秒倒数，到点响铃。做菜页自己列计时器，那里不显示浮条，但照样响铃。
 */
export default function TimerBar() {
  const t = useT();
  const { pathname } = useLocation();
  const timers = useCookTimers().data ?? [];
  const running = timers.some((x) => x.state === "running");
  const now = useNow(running);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    for (const timer of timers) {
      if (isDue(timer, now) && !rung.has(timer.id)) {
        rung.add(timer.id);
        playAlarm();
      }
      // 加时后重新开始走，下次到点还要响
      if (!isDue(timer, now) && rung.has(timer.id)) rung.delete(timer.id);
    }
  }, [timers, now]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);

  useEffect(() => {
    if (timers.length === 0) setOpen(false);
  }, [timers.length]);

  if (timers.length === 0 || /^\/recipes\/\d+\/cook\/?$/.test(pathname))
    return null;
  const sorted = sortTimers(timers, now);
  const first = sorted[0];
  const due = sorted.filter((x) => isDue(x, now)).length;
  return (
    <div className="recipes-bar" ref={ref}>
      {open && (
        <div
          className="recipes-bar-panel"
          role="dialog"
          aria-label={t("Cooking timers")}
        >
          <div className="recipes-bar-head">
            <strong>{t("Cooking timers")}</strong>
            <button
              className="xc-btn small ghost"
              aria-label={t("Collapse")}
              title={t("Collapse")}
              onClick={() => setOpen(false)}
            >
              <ChevronDown size={14} />
            </button>
          </div>
          <TimerList
            timers={timers}
            now={now}
            showLinks
            onNavigate={() => setOpen(false)}
          />
        </div>
      )}
      <button
        className={`recipes-bar-chip${due ? " due" : ""}`}
        aria-expanded={open}
        title={t("Cooking timers")}
        onClick={() => setOpen((v) => !v)}
      >
        <AlarmClock size={15} />
        <span className="recipes-bar-label">{first.label}</span>
        <span className="recipes-bar-clock">
          {isDue(first, now)
            ? t("Time is up")
            : `${first.state === "paused" ? `${t("Paused")} ` : ""}${clock(remaining(first, now))}`}
        </span>
        {timers.length > 1 && (
          <span className="recipes-bar-more">+{timers.length - 1}</span>
        )}
      </button>
    </div>
  );
}
