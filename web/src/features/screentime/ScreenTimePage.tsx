import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { ChevronLeft, ChevronRight, Hourglass, Settings } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import {
  useScreenSettings,
  useScreenSummary,
  type ScreenRange,
  type ScreenSummary,
} from "./api";
import {
  CATEGORY_COLORS,
  CATEGORY_LABELS,
  appName,
  dayLabel,
  durationText,
  localToday,
  rangeLabel,
  shiftRange,
  statValue,
} from "./format";
import SettingsDialog from "./SettingsDialog";
import "./i18n";
import "./screentime.css";

const RANGES: ScreenRange[] = ["day", "week", "month"];

/** 电脑时间去向（B116）：Windows 代理每分钟记一次前台程序。 */
export default function ScreenTimePage() {
  const t = useT();
  const language = useLanguage();
  const [params, setParams] = useSearchParams();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const rangeParam = params.get("range");
  const range: ScreenRange = RANGES.includes(rangeParam as ScreenRange)
    ? (rangeParam as ScreenRange)
    : "day";
  const date = params.get("date") ?? "";
  const host = params.get("host") ?? "";
  const set = (patch: Record<string, string>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );

  const q = useScreenSummary(range, date, host);
  const settings = useScreenSettings();
  const hosts = settings.data?.hosts ?? [];
  const s = q.data;

  const stat = (minutes: number | undefined) => {
    if (minutes === undefined) return { value: "–", unit: undefined };
    return statValue(t, minutes);
  };
  const minutesOf = (c: string) =>
    s?.categories.find((x) => x.category === c)?.minutes ?? 0;
  const total = stat(s?.minutes);
  const coding = stat(s && minutesOf("coding"));
  const ai = stat(s && minutesOf("ai"));
  const fun = stat(s && minutesOf("entertainment"));

  const atEnd = s ? s.to >= localToday() : true;
  const go = (delta: -1 | 1) => {
    if (!s) return;
    const next = shiftRange(s.from, range, delta);
    set({ date: next });
  };

  let body;
  if (q.isPending) body = <Loading />;
  else if (q.isError && isNotLive(q.error))
    body = <NotLive name={t("Screen time")} icon={<Hourglass size={28} />} />;
  else if (q.isError)
    body = <ErrorState error={q.error} onRetry={() => q.refetch()} />;
  else
    body = <Body summary={q.data} onSettings={() => setSettingsOpen(true)} />;

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Screen time")}
        subtitle={
          s && s.state === "ok" ? durationText(t, s.minutes) : undefined
        }
        aside={
          <button
            className="xc-btn"
            title={t("Screen time settings")}
            onClick={() => setSettingsOpen(true)}
          >
            <Settings size={15} /> {t("Settings")}
          </button>
        }
      />
      <StatStrip label={t("Screen time")}>
        <StatCard
          label={t("Total time")}
          value={total.value}
          unit={total.unit}
        />
        <StatCard label={t("Coding")} value={coding.value} unit={coding.unit} />
        <StatCard label={t("AI tools")} value={ai.value} unit={ai.unit} />
        <StatCard
          label={t("Entertainment")}
          value={fun.value}
          unit={fun.unit}
        />
      </StatStrip>
      <Toolbar
        start={
          <div className="screentime-nav">
            <Segmented
              label={t("Screen time")}
              value={range}
              options={RANGES.map((r) => ({
                value: r,
                label: t(r === "day" ? "Day" : r === "week" ? "Week" : "Month"),
              }))}
              onChange={(r) => set({ range: r === "day" ? "" : r, date: "" })}
            />
            <div className="screentime-pager">
              <button
                className="xc-btn small icon"
                aria-label={t("Previous period")}
                title={t("Previous period")}
                disabled={!s}
                onClick={() => go(-1)}
              >
                <ChevronLeft size={15} />
              </button>
              <span className="screentime-range">
                {s ? rangeLabel(range, s.from, s.to, language) : "…"}
              </span>
              <button
                className="xc-btn small icon"
                aria-label={t("Next period")}
                title={t("Next period")}
                disabled={!s || atEnd}
                onClick={() => go(1)}
              >
                <ChevronRight size={15} />
              </button>
            </div>
          </div>
        }
        end={
          hosts.length > 1 ? (
            <select
              className="xc-select"
              aria-label={t("Which computer")}
              value={host}
              onChange={(e) => set({ host: e.target.value })}
            >
              <option value="">{t("All computers")}</option>
              {hosts.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.name}
                </option>
              ))}
            </select>
          ) : undefined
        }
      />
      {body}
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

function Body({
  summary: s,
  onSettings,
}: {
  summary: ScreenSummary;
  onSettings: () => void;
}) {
  const t = useT();
  if (s.state === "disabled")
    return (
      <EmptyState title={t("Recording is off")} icon={<Hourglass size={28} />}>
        <span>{t("Turn it on in the settings to start recording again.")}</span>
        <button className="xc-btn primary small" onClick={onSettings}>
          {t("Open settings")}
        </button>
      </EmptyState>
    );
  if (s.state === "no_agent")
    return (
      <EmptyState
        title={t("No computer can record yet")}
        icon={<Hourglass size={28} />}
      >
        <span>
          {t(
            "Only the agent on a Windows desktop can see which program is in front. Install it on your computer and connect it.",
          )}
        </span>
        <Link className="xc-btn small" to="/pc">
          {t("Go to Computer")}
        </Link>
      </EmptyState>
    );
  if (s.state === "waiting")
    return (
      <EmptyState
        title={t("Waiting for the first minute")}
        icon={<Hourglass size={28} />}
      >
        <span>
          {t(
            "The agent is connected. The first record appears after one full minute.",
          )}
        </span>
      </EmptyState>
    );
  if (s.minutes === 0)
    return (
      <EmptyState
        title={t("No record in this period")}
        icon={<Hourglass size={28} />}
      >
        <span>{t("Nothing was recorded between these dates.")}</span>
      </EmptyState>
    );
  return (
    <div className="screentime-grid">
      <div className="screentime-col">
        <Categories summary={s} />
        {s.days.length > 0 && <Days summary={s} />}
      </div>
      <Apps summary={s} />
    </div>
  );
}

function Categories({ summary: s }: { summary: ScreenSummary }) {
  const t = useT();
  const max = s.categories[0]?.minutes ?? 0;
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Where the time went")}</h2>
      </div>
      <div className="screentime-list">
        {s.categories.map((c) => (
          <div key={c.category} className="screentime-row">
            <span className="screentime-name">
              <i style={{ background: CATEGORY_COLORS[c.category] }} />
              {t(CATEGORY_LABELS[c.category])}
            </span>
            <span className="screentime-track">
              <i
                style={{
                  width: `${max > 0 ? (c.minutes / max) * 100 : 0}%`,
                  background: CATEGORY_COLORS[c.category],
                }}
              />
            </span>
            <span className="screentime-time">
              {durationText(t, c.minutes)}
              <small>{Math.round((c.minutes / s.minutes) * 100)}%</small>
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}

function Days({ summary: s }: { summary: ScreenSummary }) {
  const t = useT();
  const language = useLanguage();
  const max = Math.max(...s.days.map((d) => d.minutes), 1);
  const month = s.range === "month";
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Time per day")}</h2>
      </div>
      <div className={`screentime-days${month ? " month" : ""}`}>
        {s.days.map((d, i) => (
          <div
            key={d.date}
            className="screentime-day"
            role="img"
            aria-label={`${d.date} ${durationText(t, d.minutes)}`}
            title={`${d.date} ${durationText(t, d.minutes)}`}
          >
            <div className="screentime-bar">
              {Object.entries(d.categories)
                .sort(
                  ([a], [b]) =>
                    categoryOrder(a as keyof typeof CATEGORY_LABELS) -
                    categoryOrder(b as keyof typeof CATEGORY_LABELS),
                )
                .map(([c, minutes]) => (
                  <i
                    key={c}
                    style={{
                      height: `${(minutes / max) * 100}%`,
                      background:
                        CATEGORY_COLORS[c as keyof typeof CATEGORY_COLORS],
                    }}
                  />
                ))}
            </div>
            <span className="screentime-day-label">
              {month && i % 5 !== 0 ? "" : dayLabel(d.date, s.range, language)}
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}

const ORDER = Object.keys(CATEGORY_LABELS);
const categoryOrder = (c: string) => ORDER.indexOf(c);

function Apps({ summary: s }: { summary: ScreenSummary }) {
  const t = useT();
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Most used programs")}</h2>
      </div>
      <div className="screentime-apps">
        {s.apps.map((a) => (
          <div key={a.app} className="screentime-app">
            <i style={{ background: CATEGORY_COLORS[a.category] }} />
            <span className="screentime-app-main">
              <strong>{appName(a.app)}</strong>
              <small>{t(CATEGORY_LABELS[a.category])}</small>
            </span>
            <span className="screentime-time">
              {durationText(t, a.minutes)}
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}
