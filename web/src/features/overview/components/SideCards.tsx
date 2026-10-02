import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  CloudSun,
  Droplets,
  Plus,
  RefreshCw,
  Umbrella,
} from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useMarkNotificationRead, useNotifications } from "../../../api/core";
import { Ring } from "../../../components/ui/Stat";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatTime, relativeTime } from "../../../lib/time";
import { useCalendarEvents, useWeatherExtra } from "../../calendar/api";
import { useNow } from "../../calendar/hooks";
import { addDays, startOfDay } from "../../calendar/dates";
import { useCheckin, useHabitsToday, useUndoCheckin } from "../../habits/api";
import { formatAmount } from "../../habits/progress";
import { useCallService, useHAFavorites } from "../../home/api";
import EntityIcon from "../../home/components/EntityIcon";
import {
  displayName,
  isOn,
  isUnavailable,
  stateLabel,
  tapAction,
} from "../../home/logic";
import { refreshWeather, useWeather } from "../api";
import { sortEvents } from "../today";
import { Empty, isSetupNeeded, MoreLink, QueryState } from "./shared";
import WeatherDialog, { loadWeatherShow } from "./WeatherDialog";
import { weatherIcon } from "../weatherIcon";
import WeatherDetail from "./WeatherDetail";
import {
  airTone,
  rainSoon,
  rainSoonText,
  sortWarnings,
  warningLabel,
  warningTone,
} from "../weather";

const onError = (e: unknown) =>
  toast({ message: errorMessage(e), tone: "error" });

/** 今天的日程，全天事件在前。 */
export function ScheduleCard() {
  const t = useT();
  const language = useLanguage();
  const from = startOfDay(new Date());
  const events = useCalendarEvents(from, addDays(from, 1));
  if (events.isPending || events.isError) return <QueryState query={events} />;
  const list = sortEvents(events.data);
  if (list.length === 0)
    return (
      <Empty action={<MoreLink to="/calendar" label={t("Open calendar")} />}>
        {t("No events today")}
      </Empty>
    );
  const now = Date.now();
  return (
    <div className="xc-list">
      {list.map((e) => {
        const past = !e.allDay && new Date(e.end).getTime() < now;
        return (
          <Link
            key={e.id}
            className={`today-row today-event${past ? " is-past" : ""}`}
            to="/calendar"
          >
            <span className="today-event-time">
              {e.allDay ? (
                t("All day")
              ) : (
                <>
                  {formatTime(e.start, language)}
                  <small>{formatTime(e.end, language)}</small>
                </>
              )}
            </span>
            <i
              className="today-event-bar"
              style={{ background: e.color || undefined }}
            />
            <span className="today-row-main">
              <strong>{e.title}</strong>
              <small>{e.location || e.calendar}</small>
            </span>
          </Link>
        );
      })}
    </div>
  );
}

/** 今天的习惯进度，每行能直接打卡。 */
export function HabitsCard() {
  const t = useT();
  const language = useLanguage();
  const today = useHabitsToday();
  const checkin = useCheckin();
  const undo = useUndoCheckin();
  if (today.isPending || today.isError) return <QueryState query={today} />;
  if (today.data.length === 0)
    return (
      <Empty action={<MoreLink to="/habits?new=1" label={t("Add habit")} />}>
        {t("No habits yet")}
      </Empty>
    );
  return (
    <div className="xc-list">
      {today.data.map((p) => {
        const h = p.habit;
        return (
          <div className="today-row" key={h.id}>
            <Ring
              value={p.done}
              max={h.dailyTarget}
              size={30}
              stroke={4}
              tone={p.reached ? "ok" : "accent"}
            />
            <Link className="today-row-main" to="/habits">
              <strong>
                {h.icon && <span className="today-habit-icon">{h.icon}</span>}
                {h.name}
              </strong>
              <small>
                {formatAmount(p.done)} / {formatAmount(h.dailyTarget)} {h.unit}
                {p.streak > 0 && (
                  <>
                    {" "}
                    · {p.streak} {t("days")}
                  </>
                )}
                {h.nextRemindAt && !p.reached && (
                  <>
                    {" "}
                    · {t("Next reminder at")}{" "}
                    {relativeTime(h.nextRemindAt, language)}
                  </>
                )}
              </small>
            </Link>
            <button
              className="xc-btn small ghost"
              title={t("Check in")}
              aria-label={`${t("Check in")} ${h.name}`}
              disabled={checkin.isPending}
              onClick={() =>
                checkin.mutate(
                  { id: h.id },
                  {
                    onSuccess: (res) =>
                      toast({
                        message: t("Checked in"),
                        subtitle: `${h.name} ${formatAmount(res.today.done)}/${formatAmount(h.dailyTarget)} ${h.unit}`,
                        onUndo: () => undo.mutate(res.log.id, { onError }),
                      }),
                    onError,
                  },
                )
              }
            >
              <Plus size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}

/** 当前天气和今天的最高最低温。 */
/** 标题右下的一行天气。点一下打开天气设置。 */
export function WeatherStrip() {
  const t = useT();
  const language = useLanguage();
  const weather = useWeather();
  const qc = useQueryClient();
  const [open, setOpen] = useState<"detail" | "settings" | null>(null);
  const [spinning, setSpinning] = useState(false);
  const [show, setShow] = useState(loadWeatherShow);
  // B58：和风天气的预警、两小时降水、空气质量
  const extra = useWeatherExtra().data;
  const now = useNow(60_000);
  useEffect(() => {
    const onChange = () => setShow(loadWeatherShow());
    window.addEventListener("xc:weather-show", onChange);
    return () => window.removeEventListener("xc:weather-show", onChange);
  }, []);
  if (weather.isPending) return null;
  const setup = weather.isError && isSetupNeeded(weather.error);
  if (weather.isError && !setup) return null;
  const w = weather.data;
  const soon = rainSoon(extra?.minutely, now);
  // 最多显示两条，颜色重的在前
  const warnings = sortWarnings(extra?.warnings ?? []).slice(0, 2);
  const refresh = async () => {
    if (spinning) return;
    setSpinning(true);
    // 至少转一圈，太快看不出点过了。
    const turn = new Promise((r) => setTimeout(r, 700));
    try {
      await Promise.all([refreshWeather(qc), turn]);
    } catch (err) {
      toast({ message: errorMessage(err), tone: "error" });
    } finally {
      setSpinning(false);
    }
  };
  // 鼠标移到整个天气上时显示更新时间。
  const updated = w?.fetchedAt
    ? `${t("Updated at")} ${formatTime(w.fetchedAt, language)}（${relativeTime(w.fetchedAt, language)}）`
    : undefined;
  return (
    <span className="today-weather" title={updated}>
      <button
        type="button"
        className={`today-weather-strip${setup ? " is-setup" : ""}`}
        onClick={() => setOpen(setup || !w ? "settings" : "detail")}
      >
        {w && !setup ? (
          <WeatherGlyph weather={w} size={14} />
        ) : (
          <CloudSun size={14} />
        )}
        {setup || !w ? (
          <span>
            {t("Weather")} · {t("Set a place")}
          </span>
        ) : (
          <>
            {show.place && w.location && <span>{w.location}</span>}
            <b>{Math.round(w.temperature)}°</b>
            <span>{w.summary}</span>
            {show.range && (
              <span className="today-weather-range">
                {Math.round(w.low)}°/{Math.round(w.high)}°
              </span>
            )}
            {/* B90：湿度、空气质量、降水概率、降水提醒，按这个顺序 */}
            {show.humidity && w.humidity != null && (
              <span className="today-weather-rain" title={t("Humidity")}>
                <Droplets size={12} /> {w.humidity}%
              </span>
            )}
            {show.air && extra?.air && (
              <span
                className={`weather-tone-${airTone(extra.air.level)}`}
                title={`${t("Air quality")} ${extra.air.aqi}`}
              >
                {extra.air.category}
              </span>
            )}
            {show.rain && (
              <span
                className="today-weather-rain"
                title={t("Chance of rain today")}
              >
                <Umbrella size={12} /> {w.precipitationChance}%
              </span>
            )}
            {soon && (
              <span className="today-weather-soon">{rainSoonText(soon)}</span>
            )}
          </>
        )}
      </button>
      {warnings.map((x) => (
        <button
          type="button"
          key={x.id}
          className={`xc-badge ${warningTone(x.level)} today-weather-warning`}
          title={x.title}
          onClick={() => setOpen("detail")}
        >
          {warningLabel(x)}
        </button>
      ))}
      {w && (
        <button
          type="button"
          className={`today-weather-refresh${spinning ? " is-spinning" : ""}`}
          aria-label={t("Refresh weather")}
          disabled={spinning}
          onClick={refresh}
        >
          <RefreshCw size={13} />
        </button>
      )}
      {open === "detail" && (
        <WeatherDetail
          weather={w}
          onClose={() => setOpen(null)}
          onSettings={() => setOpen("settings")}
        />
      )}
      <WeatherDialog open={open === "settings"} onClose={() => setOpen(null)} />
    </span>
  );
}

/** B90：按天气选的图标。 */
function WeatherGlyph({
  weather,
  size,
}: {
  weather: Parameters<typeof weatherIcon>[0];
  size: number;
}) {
  const Icon = weatherIcon(weather);
  return <Icon size={size} />;
}

/** Home Assistant 收藏的设备，点一下开关。 */
export function HomeCard() {
  const t = useT();
  const favorites = useHAFavorites();
  const call = useCallService();
  if (favorites.isPending || favorites.isError)
    return <QueryState query={favorites} setupTo="/settings/homeassistant" />;
  if (favorites.data.length === 0)
    return (
      <Empty action={<MoreLink to="/home" label={t("Browse devices")} />}>
        {t("No favorites yet")}
      </Empty>
    );
  return (
    <div className="today-home">
      {favorites.data.slice(0, 8).map((f) => {
        const action = tapAction(f.entityId, f.state);
        const off = isUnavailable(f.state);
        const on = f.state ? isOn(f.state) : false;
        const name = displayName(f.entityId, f.state, f.alias);
        return (
          <button
            key={f.entityId}
            className={`today-home-tile${on ? " is-on" : ""}`}
            disabled={off || action.kind === "none" || call.isPending}
            title={name}
            onClick={() =>
              action.kind !== "none" &&
              call.mutate(
                {
                  domain: action.domain,
                  service: action.service,
                  entityId: f.entityId,
                },
                { onError },
              )
            }
          >
            <EntityIcon entityId={f.entityId} state={f.state} size={16} />
            <span>
              <strong>{name}</strong>
              <small>
                {f.state ? t(stateLabel(f.state)) : t("Unavailable")}
              </small>
            </span>
          </button>
        );
      })}
    </div>
  );
}

/** 最近的站内通知。 */
export function ActivityCard() {
  const t = useT();
  const language = useLanguage();
  const list = useNotifications();
  const read = useMarkNotificationRead();
  if (list.isPending || list.isError) return <QueryState query={list} />;
  const items = list.data.items.slice(0, 8);
  if (items.length === 0) return <Empty>{t("No notifications")}</Empty>;
  return (
    <div className="xc-list">
      {items.map((n) => {
        const body = (
          <>
            <span className={`today-row-icon${n.readAt ? "" : " accent"}`}>
              <Bell size={14} />
            </span>
            <span className="today-row-main">
              <strong>{n.title}</strong>
              {n.body && <small>{n.body}</small>}
            </span>
            <time className="today-row-time">
              {relativeTime(n.createdAt, language)}
            </time>
          </>
        );
        const markRead = () => !n.readAt && read.mutate(n.id);
        return n.link ? (
          <Link key={n.id} className="today-row" to={n.link} onClick={markRead}>
            {body}
          </Link>
        ) : (
          <div key={n.id} className="today-row">
            {body}
          </div>
        );
      })}
    </div>
  );
}
