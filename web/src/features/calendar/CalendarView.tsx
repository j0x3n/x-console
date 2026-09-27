import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import {
  CalendarPlus,
  ChevronLeft,
  ChevronRight,
  MapPin,
  Pencil,
  Plus,
  Repeat,
} from "lucide-react";
import EventForm from "./EventForm";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Spinner } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { formatTime } from "../../lib/time";
import { useCalendarEvents, useCalendars, type CalendarEvent } from "./api";
import {
  addDays,
  allDayEvents,
  dayKey,
  layoutDay,
  parseDayKey,
  rangeTitle,
  viewRange,
  type CalendarView as View,
} from "./dates";
import { useMediaQuery, useNow } from "./hooks";

const HOUR = 48; // 每小时的高度，像素

export default function CalendarView() {
  const t = useT();
  const language = useLanguage();
  const phone = useMediaQuery("(max-width: 640px)");
  const [params, setParams] = useSearchParams();
  const view: View =
    params.get("view") === "day" || params.get("view") === "week"
      ? (params.get("view") as View)
      : phone
        ? "day"
        : "week";
  const anchorKey = params.get("date");
  const anchor = useMemo(
    () => parseDayKey(anchorKey) ?? new Date(),
    [anchorKey],
  );
  const { from, to, days } = useMemo(
    () => viewRange(view, anchor),
    [view, anchor],
  );
  const events = useCalendarEvents(from, to);
  const calendars = useCalendars();
  const [selected, setSelected] = useState<CalendarEvent | null>(null);
  const [form, setForm] = useState<{
    event: CalendarEvent | null;
    start: Date | null;
  } | null>(null);
  const navigate = useNavigate();
  const writable = (calendars.data ?? []).filter(
    (c) => c.enabled && (c.writable ?? c.kind !== "ics"),
  );
  const newEvent = (start: Date | null) => {
    if (writable.length === 0) {
      navigate("/calendar/calendars?new=local");
      return;
    }
    setForm({ event: null, start });
  };

  const setParam = (changes: Record<string, string | null>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(changes)) {
      if (v === null) next.delete(k);
      else next.set(k, v);
    }
    setParams(next, { replace: true });
  };
  const step = view === "week" ? 7 : 1;
  const go = (n: number) =>
    setParam({ date: dayKey(addDays(anchor, n * step)) });

  return (
    <div className="calendar-view">
      <div className="calendar-toolbar">
        <div className="xc-row">
          <button
            className="xc-btn small"
            onClick={() => go(-1)}
            aria-label={t("Previous")}
          >
            <ChevronLeft size={15} />
          </button>
          <button
            className="xc-btn small"
            onClick={() => setParam({ date: null })}
          >
            {t("Today")}
          </button>
          <button
            className="xc-btn small"
            onClick={() => go(1)}
            aria-label={t("Next")}
          >
            <ChevronRight size={15} />
          </button>
          <strong className="calendar-range">
            {rangeTitle(view, days, language)}
          </strong>
          {events.isFetching && <Spinner />}
        </div>
        <div className="xc-row">
          <button
            className="xc-btn small primary"
            onClick={() => newEvent(null)}
            title={t("New event")}
          >
            <Plus size={14} />{" "}
            <span className="calendar-btn-text">{t("New event")}</span>
          </button>
          <div className="calendar-switch" role="group" aria-label={t("View")}>
            {(["day", "week"] as const).map((v) => (
              <button
                key={v}
                className={v === view ? "active" : ""}
                aria-pressed={v === view}
                onClick={() => setParam({ view: v })}
              >
                {t(v === "day" ? "Day" : "Week")}
              </button>
            ))}
          </div>
        </div>
      </div>

      {calendars.data?.length === 0 && (
        <div className="xc-card calendar-hint">
          <CalendarPlus size={16} />
          <span>
            {t("No calendars yet. Add an ICS link or a CalDAV account.")}
          </span>
          <Link className="xc-btn small primary" to="/calendar/calendars?new=1">
            {t("Add calendar")}
          </Link>
        </div>
      )}

      {events.isError ? (
        <ErrorState error={events.error} onRetry={() => events.refetch()} />
      ) : (
        <TimeGrid
          days={days}
          events={events.data ?? []}
          onSelect={setSelected}
          onPick={newEvent}
          language={language}
        />
      )}
      <EventDialog
        event={selected}
        onClose={() => setSelected(null)}
        onEdit={(e) => {
          setSelected(null);
          setForm({ event: e, start: null });
        }}
      />
      <EventForm
        open={form !== null}
        event={form?.event ?? null}
        start={form?.start ?? null}
        calendars={writable}
        onClose={() => setForm(null)}
      />
    </div>
  );
}

function TimeGrid({
  days,
  events,
  onSelect,
  onPick,
  language,
}: {
  days: Date[];
  events: CalendarEvent[];
  onSelect: (e: CalendarEvent) => void;
  onPick: (start: Date) => void;
  language: "zh" | "en";
}) {
  const t = useT();
  const now = useNow(60_000);
  const scrollRef = useRef<HTMLDivElement>(null);
  const todayKey = dayKey(now);
  const locale = language === "zh" ? "zh-CN" : "en";

  // 打开时滚到 7 点，或者更早的第一个事件。
  const firstHour = useMemo(() => {
    let h = 7;
    for (const d of days) {
      for (const b of layoutDay(events, d))
        h = Math.min(h, Math.floor(b.top / 60));
    }
    return h;
  }, [days, events]);
  const daysKey = days.map(dayKey).join(",");
  useEffect(() => {
    if (scrollRef.current)
      scrollRef.current.scrollTop = Math.max(0, firstHour * HOUR - 8);
    // 只在换日期范围时滚动，事件刷新时不打扰。
  }, [daysKey]);

  const hasAllDay = days.some((d) => allDayEvents(events, d).length > 0);
  const style = { "--calendar-days": days.length } as CSSProperties;

  return (
    <div
      className={`calendar-grid-wrap${days.length > 1 ? " is-week" : ""}`}
      ref={scrollRef}
    >
      <div className="calendar-grid" style={style}>
        <div className="calendar-top">
          <div className="calendar-row calendar-head">
            <div className="calendar-gutter" />
            {days.map((d) => (
              <div
                key={dayKey(d)}
                className={`calendar-dayhead${dayKey(d) === todayKey ? " is-today" : ""}`}
              >
                <span>
                  {d.toLocaleDateString(locale, { weekday: "short" })}
                </span>
                <b>{d.getDate()}</b>
              </div>
            ))}
          </div>
          {hasAllDay && (
            <div className="calendar-row calendar-allday">
              <div className="calendar-gutter">{t("All day")}</div>
              {days.map((d) => (
                <div key={dayKey(d)} className="calendar-allday-cell">
                  {allDayEvents(events, d).map((e) => (
                    <button
                      key={e.id}
                      className="calendar-chip"
                      style={{ "--ev": e.color || undefined } as CSSProperties}
                      onClick={() => onSelect(e)}
                      title={e.title}
                    >
                      {e.title || t("(no title)")}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          )}
        </div>
        <div
          className="calendar-row calendar-body"
          style={{ height: 24 * HOUR }}
        >
          <div className="calendar-gutter calendar-hours">
            {Array.from({ length: 23 }, (_, i) => (
              <span key={i} style={{ top: (i + 1) * HOUR }}>
                {String(i + 1).padStart(2, "0")}:00
              </span>
            ))}
          </div>
          {days.map((d) => (
            <DayColumn
              key={dayKey(d)}
              day={d}
              events={events}
              now={dayKey(d) === todayKey ? now : null}
              onSelect={onSelect}
              onPick={onPick}
              language={language}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function DayColumn({
  day,
  events,
  now,
  onSelect,
  onPick,
  language,
}: {
  day: Date;
  events: CalendarEvent[];
  now: Date | null;
  onSelect: (e: CalendarEvent) => void;
  onPick: (start: Date) => void;
  language: "zh" | "en";
}) {
  const t = useT();
  const blocks = useMemo(() => layoutDay(events, day), [events, day]);
  const nowTop = now
    ? ((now.getHours() * 60 + now.getMinutes()) / 60) * HOUR
    : null;
  return (
    <div
      className={`calendar-col${now ? " is-today" : ""}`}
      onClick={(ev) => {
        // 点空白处新建，按半小时取整
        if (ev.target !== ev.currentTarget) return;
        const y = ev.clientY - ev.currentTarget.getBoundingClientRect().top;
        const minutes = Math.floor(((y / HOUR) * 60) / 30) * 30;
        const start = new Date(day);
        start.setHours(0, minutes, 0, 0);
        onPick(start);
      }}
    >
      {blocks.map((b) => {
        const e = b.event;
        const style = {
          top: (b.top / 60) * HOUR,
          height: Math.max((b.height / 60) * HOUR - 2, 14),
          left: `calc(${(b.column / b.columns) * 100}% + 2px)`,
          width: `calc(${100 / b.columns}% - 4px)`,
          "--ev": e.color || undefined,
        } as CSSProperties;
        const short = b.height < 45;
        return (
          <button
            key={e.id}
            className={`calendar-event${short ? " is-short" : ""}`}
            style={style}
            onClick={() => onSelect(e)}
            title={`${formatTime(e.start, language)} ${e.title}`}
          >
            <strong>{e.title || t("(no title)")}</strong>
            <span>
              {b.continuesBefore ? "…" : formatTime(e.start, language)}
              {!short &&
                ` – ${b.continuesAfter ? "…" : formatTime(e.end, language)}`}
            </span>
          </button>
        );
      })}
      {nowTop !== null && (
        <div className="calendar-now" style={{ top: nowTop }} />
      )}
    </div>
  );
}

function EventDialog({
  event,
  onClose,
  onEdit,
}: {
  event: CalendarEvent | null;
  onClose: () => void;
  onEdit: (e: CalendarEvent) => void;
}) {
  const t = useT();
  const language = useLanguage();
  if (!event) return null;
  const locale = language === "zh" ? "zh-CN" : "en";
  const date = (iso: string) =>
    new Date(iso).toLocaleDateString(locale, {
      month: "short",
      day: "numeric",
      weekday: "short",
    });
  let when: string;
  if (event.allDay) {
    const last = addDays(parseDayKey(event.endDate) ?? new Date(event.end), -1);
    const first = event.startDate ?? dayKey(new Date(event.start));
    when =
      dayKey(last) === first
        ? `${date(event.start)} · ${t("All day")}`
        : `${date(event.start)} – ${date(last.toISOString())} · ${t("All day")}`;
  } else {
    const sameDay =
      dayKey(new Date(event.start)) === dayKey(new Date(event.end));
    when = sameDay
      ? `${date(event.start)} ${formatTime(event.start, language)} – ${formatTime(event.end, language)}`
      : `${date(event.start)} ${formatTime(event.start, language)} – ${date(event.end)} ${formatTime(event.end, language)}`;
  }
  return (
    <Dialog
      open
      onClose={onClose}
      title={event.title || t("(no title)")}
      footer={
        <>
          {event.writable && (
            <button className="xc-btn" onClick={() => onEdit(event)}>
              <Pencil size={14} /> {t("Edit")}
            </button>
          )}
          <button className="xc-btn" onClick={onClose}>
            {t("Close")}
          </button>
        </>
      }
    >
      <div className="calendar-detail">
        <p>{when}</p>
        {event.location && (
          <p>
            <MapPin size={14} /> {event.location}
          </p>
        )}
        <p className="xc-muted">
          <span
            className="calendar-dot"
            style={{ "--ev": event.color || undefined } as CSSProperties}
          />
          {event.calendar}
          {event.recurring && (
            <>
              {" · "}
              <Repeat size={13} /> {t("Repeats")}
            </>
          )}
        </p>
        {event.description && (
          <pre className="calendar-desc">{event.description}</pre>
        )}
      </div>
    </Dialog>
  );
}
