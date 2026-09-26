import { useState } from "react";
import { Link } from "react-router";
import { Play, Timer } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { formatTime, relativeTime } from "../../lib/time";
import { useFocusStats } from "./api";
import { dayKey, formatDuration, parseDayKey } from "./dates";
import { issuePath } from "../projects/logic";
import { useFocusPanel } from "./hooks";

const ranges = [7, 30];

/** 番茄钟统计：每天专注多久、花在哪些 Issue 上、最近几次。 */
export default function FocusPage() {
  const t = useT();
  const language = useLanguage();
  const [days, setDays] = useState(7);
  const stats = useFocusStats(days);
  const openFor = useFocusPanel((s) => s.openFor);

  if (stats.isPending) return <Loading />;
  if (stats.isError)
    return <ErrorState error={stats.error} onRetry={() => stats.refetch()} />;
  const data = stats.data;
  const today = data.days.find((d) => d.date === dayKey(new Date()));
  const max = Math.max(1, ...data.days.map((d) => d.seconds));
  const locale = language === "zh" ? "zh-CN" : "en";

  return (
    <div className="xc-stack">
      <div className="xc-row">
        <div className="calendar-switch" role="group" aria-label={t("Range")}>
          {ranges.map((n) => (
            <button
              key={n}
              className={n === days ? "active" : ""}
              aria-pressed={n === days}
              onClick={() => setDays(n)}
            >
              {n} {t("days")}
            </button>
          ))}
        </div>
        <span className="xc-spacer" />
        <button className="xc-btn primary small" onClick={() => openFor("")}>
          <Play size={14} /> {t("Start focus")}
        </button>
      </div>

      <div className="focus-tiles">
        <div className="xc-card focus-tile">
          <span>{t("Today")}</span>
          <strong>{formatDuration(today?.seconds ?? 0, language)}</strong>
        </div>
        <div className="xc-card focus-tile">
          <span>
            {t("Last")} {days} {t("days")}
          </span>
          <strong>{formatDuration(data.totalSeconds, language)}</strong>
        </div>
        <div className="xc-card focus-tile">
          <span>{t("Completed sessions")}</span>
          <strong>{data.completed}</strong>
        </div>
      </div>

      <section className="xc-card focus-chart-card">
        <div className="xc-card-head">
          <h3>{t("Focus per day")}</h3>
        </div>
        <div className={`focus-chart${days > 7 ? " is-dense" : ""}`}>
          {data.days.map((d) => {
            const date = parseDayKey(d.date);
            const label = date
              ? days > 7
                ? String(date.getDate())
                : date.toLocaleDateString(locale, { weekday: "short" })
              : d.date;
            return (
              <div
                key={d.date}
                className="focus-chart-col"
                title={`${d.date} · ${formatDuration(d.seconds, language)}`}
              >
                <div className="focus-chart-bar">
                  <i style={{ height: `${(d.seconds / max) * 100}%` }} />
                </div>
                <span>{label}</span>
              </div>
            );
          })}
        </div>
      </section>

      <div className="focus-columns">
        <section className="xc-card">
          <div className="xc-card-head">
            <h3>{t("By issue")}</h3>
          </div>
          {data.byIssue.length === 0 ? (
            <p className="xc-muted focus-empty">
              {t("No focus linked to an issue yet.")}
            </p>
          ) : (
            <ul className="focus-list">
              {data.byIssue.map((i) => (
                <li key={i.issueKey}>
                  <Link className="xc-mono" to={issuePath(i.issueKey)}>
                    {i.issueKey}
                  </Link>
                  <span className="xc-muted">
                    {formatDuration(i.seconds, language)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="xc-card">
          <div className="xc-card-head">
            <h3>{t("Recent")}</h3>
          </div>
          {data.recent.length === 0 ? (
            <EmptyState
              title={t("No focus sessions yet")}
              icon={<Timer size={24} />}
            />
          ) : (
            <ul className="focus-list">
              {data.recent.map((s) => (
                <li key={s.id}>
                  <span>
                    <span className={`xc-badge ${s.completed ? "ok" : ""}`}>
                      {s.completed ? t("Finished") : t("Stopped early")}
                    </span>{" "}
                    {formatTime(s.startedAt, language)} ·{" "}
                    {formatDuration(s.actualSeconds, language)}
                    {s.issueKey && (
                      <>
                        {" · "}
                        <Link className="xc-mono" to={issuePath(s.issueKey)}>
                          {s.issueKey}
                        </Link>
                      </>
                    )}
                  </span>
                  <span className="xc-muted">
                    {relativeTime(s.startedAt, language)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}
