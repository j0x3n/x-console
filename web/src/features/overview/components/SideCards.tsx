import { Link } from "react-router";
import { Bell, CloudSun, Droplets, Plus } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useMarkNotificationRead, useNotifications } from "../../../api/core";
import { Ring } from "../../../components/ui/Stat";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatTime, relativeTime } from "../../../lib/time";
import { useCalendarEvents } from "../../calendar/api";
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
import { useWeather } from "../api";
import { sortEvents } from "../today";
import { Empty, isSetupNeeded, MoreLink, QueryState } from "./shared";

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
/** 标题下面的一行天气。没设置位置时给一个“去设置”。 */
export function WeatherStrip() {
  const t = useT();
  const weather = useWeather();
  if (weather.isPending) return null;
  if (weather.isError)
    return isSetupNeeded(weather.error) ? (
      <Link className="today-weather-strip is-setup" to="/settings/brief">
        <CloudSun size={14} /> {t("Weather")} · {t("Go to settings")}
      </Link>
    ) : null;
  const w = weather.data;
  return (
    <span className="today-weather-strip" title={w.location || undefined}>
      <CloudSun size={14} />
      <b>{Math.round(w.temperature)}°</b>
      <span>{w.summary}</span>
      <span className="today-weather-range">
        {Math.round(w.low)}°/{Math.round(w.high)}°
      </span>
      <span className="today-weather-rain">
        <Droplets size={12} /> {w.precipitationChance}%
      </span>
    </span>
  );
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
