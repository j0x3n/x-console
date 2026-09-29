import { useEffect, useState, type FormEvent } from "react";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateEvent,
  useDeleteEvent,
  useUpdateEvent,
  type Calendar,
  type CalendarEvent,
  type EventInput,
} from "./api";
import { addDays, dayKey, parseDayKey } from "./dates";
import { confirmAction } from "../../components/ui/ConfirmDialog";

const hm = (d: Date) =>
  `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;

interface Draft {
  title: string;
  calendarId: number;
  allDay: boolean;
  date: string;
  endDate: string; // 全天事件的最后一天（含）
  startTime: string;
  endTime: string;
  location: string;
  description: string;
}

function toDraft(
  event: CalendarEvent | null,
  start: Date | null,
  calendars: Calendar[],
): Draft {
  if (event) {
    const s = new Date(event.start);
    const e = new Date(event.end);
    const last = event.allDay
      ? addDays(parseDayKey(event.endDate) ?? e, -1)
      : e;
    return {
      title: event.title,
      calendarId: event.calendarId,
      allDay: event.allDay,
      date: event.startDate ?? dayKey(s),
      endDate: dayKey(last),
      startTime: hm(s),
      endTime: hm(e),
      location: event.location,
      description: event.description,
    };
  }
  const s = start ?? nextHalfHour(new Date());
  const e = new Date(s.getTime() + 60 * 60_000);
  return {
    title: "",
    calendarId: calendars[0]?.id ?? 0,
    allDay: false,
    date: dayKey(s),
    endDate: dayKey(s),
    startTime: hm(s),
    endTime: hm(e),
    location: "",
    description: "",
  };
}

function nextHalfHour(d: Date) {
  const out = new Date(d);
  out.setSeconds(0, 0);
  out.setMinutes(out.getMinutes() < 30 ? 30 : 60);
  return out;
}

/** 把表单变成接口要的请求体。时间不对时返回错误文字（英文原文）。 */
export function draftToInput(d: Draft): EventInput | string {
  if (!d.title.trim()) return "Title is required";
  if (!d.calendarId) return "Choose a calendar";
  const base = {
    calendarId: d.calendarId,
    title: d.title.trim(),
    location: d.location.trim(),
    description: d.description.trim(),
  };
  if (d.allDay) {
    if (d.endDate < d.date) return "The end must be after the start";
    const end = addDays(parseDayKey(d.endDate)!, 1);
    return {
      ...base,
      allDay: true,
      startDate: d.date,
      endDate: dayKey(end),
    };
  }
  const start = new Date(`${d.date}T${d.startTime}`);
  const end = new Date(`${d.date}T${d.endTime}`);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()))
    return "Enter a valid time";
  if (end <= start) return "The end must be after the start";
  return {
    ...base,
    allDay: false,
    start: start.toISOString(),
    end: end.toISOString(),
  };
}

/** 新建或修改一个日程。event 为空时是新建，start 是点到的时间。 */
export default function EventForm({
  open,
  event,
  start,
  calendars,
  onClose,
}: {
  open: boolean;
  event: CalendarEvent | null;
  start: Date | null;
  calendars: Calendar[];
  onClose: () => void;
}) {
  const t = useT();
  const create = useCreateEvent();
  const update = useUpdateEvent();
  const remove = useDeleteEvent();
  const [d, setD] = useState<Draft>(() => toDraft(event, start, calendars));
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    setD(toDraft(event, start, calendars));
    setError("");
    // 只在打开时重置
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, event, start]);
  const set = (patch: Partial<Draft>) =>
    setD((prev) => ({ ...prev, ...patch }));
  const pending = create.isPending || update.isPending || remove.isPending;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const body = draftToInput(d);
    if (typeof body === "string") return setError(t(body));
    try {
      if (event) await update.mutateAsync({ id: event.eventId, body });
      else await create.mutateAsync(body);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };
  const del = async () => {
    if (!event || !(await confirmAction({ title: t("Delete this event?") })))
      return;
    try {
      await remove.mutateAsync(event.eventId);
      toast(t("Deleted"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={event ? t("Edit event") : t("New event")}
    >
      <form onSubmit={submit} className="calendar-form">
        <label className="xc-field">
          <span>{t("Title")}</span>
          <input
            className="xc-input"
            value={d.title}
            autoFocus
            maxLength={200}
            onChange={(e) => set({ title: e.target.value })}
          />
        </label>
        <label className="xc-field">
          <span>{t("Calendar")}</span>
          <select
            className="xc-select"
            value={d.calendarId}
            onChange={(e) => set({ calendarId: Number(e.target.value) })}
          >
            {calendars.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={d.allDay}
            onChange={(e) => set({ allDay: e.target.checked })}
          />
          <span>{t("All day")}</span>
        </label>
        {d.allDay ? (
          <div className="calendar-form-row">
            <label className="xc-field">
              <span>{t("Starts")}</span>
              <input
                className="xc-input"
                type="date"
                value={d.date}
                onChange={(e) =>
                  set({
                    date: e.target.value,
                    endDate:
                      d.endDate < e.target.value ? e.target.value : d.endDate,
                  })
                }
              />
            </label>
            <label className="xc-field">
              <span>{t("Ends")}</span>
              <input
                className="xc-input"
                type="date"
                value={d.endDate}
                min={d.date}
                onChange={(e) => set({ endDate: e.target.value })}
              />
            </label>
          </div>
        ) : (
          <div className="calendar-form-row is-time">
            <label className="xc-field">
              <span>{t("Date")}</span>
              <input
                className="xc-input"
                type="date"
                value={d.date}
                onChange={(e) => set({ date: e.target.value })}
              />
            </label>
            <label className="xc-field">
              <span>{t("Starts")}</span>
              <input
                className="xc-input"
                type="time"
                value={d.startTime}
                onChange={(e) => set({ startTime: e.target.value })}
              />
            </label>
            <label className="xc-field">
              <span>{t("Ends")}</span>
              <input
                className="xc-input"
                type="time"
                value={d.endTime}
                onChange={(e) => set({ endTime: e.target.value })}
              />
            </label>
          </div>
        )}
        <label className="xc-field">
          <span>{t("Location")}</span>
          <input
            className="xc-input"
            value={d.location}
            onChange={(e) => set({ location: e.target.value })}
          />
        </label>
        <div className="xc-field">
          <span>{t("Event notes")}</span>
          <MarkdownEditor
            uploadScope="calendar"
            label={t("Event notes")}
            value={d.description}
            onChange={(v) => set({ description: v })}
            minRows={3}
          />
        </div>
        {error && <p className="calendar-form-error">{error}</p>}
        <div className="xc-dialog-actions">
          {event && (
            <button
              type="button"
              className="xc-btn ghost danger calendar-form-delete"
              disabled={pending}
              onClick={del}
            >
              <Trash2 size={14} /> {t("Delete")}
            </button>
          )}
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={pending}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
