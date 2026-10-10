import { useEffect, useState, type FormEvent } from "react";
import { Plus, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateContact,
  useUpdateContact,
  type ContactEventKind,
  type ContactGroup,
  type ContactItem,
} from "./api";
import {
  EVENT_KINDS,
  EVENT_KIND_LABELS,
  GROUPS,
  GROUP_LABELS,
  formatDays,
  parseDays,
  today,
  validDate,
} from "./format";

interface Props {
  open: boolean;
  onClose: () => void;
  /** 传了就是修改 */
  contact?: ContactItem | null;
  /** 新建时默认的分组 */
  defaultGroup?: ContactGroup;
}

interface EventDraft {
  id?: string;
  kind: ContactEventKind;
  label: string;
  date: string;
}

const KIND_DEFAULT_LABEL: Record<ContactEventKind, string> = {
  birthday: "生日",
  anniversary: "纪念日",
  other: "日子",
};

/** 新建或修改一个联系人，包括他的重要日期。 */
export default function ContactDialog({
  open,
  onClose,
  contact: c,
  defaultGroup,
}: Props) {
  const t = useT();
  const create = useCreateContact();
  const update = useUpdateContact();
  const [name, setName] = useState("");
  const [group, setGroup] = useState<ContactGroup>("other");
  const [events, setEvents] = useState<EventDraft[]>([]);
  const [lastContact, setLastContact] = useState("");
  const [every, setEvery] = useState("");
  const [remind, setRemind] = useState("7, 1");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setName(c?.name ?? "");
    setGroup(c?.group ?? defaultGroup ?? "other");
    setEvents(
      c?.events.map((e) => ({
        id: e.id,
        kind: e.kind,
        label: e.label,
        date: e.date,
      })) ?? [],
    );
    setLastContact(c?.lastContactOn ?? "");
    setEvery(c?.contactEveryDays ? String(c.contactEveryDays) : "");
    setRemind(c ? formatDays(c.remindDays) : "7, 1");
    setNotes(c?.notes ?? "");
  }, [open, c, defaultGroup]);

  const saving = create.isPending || update.isPending;
  const patch = (i: number, p: Partial<EventDraft>) =>
    setEvents((list) => list.map((e, j) => (j === i ? { ...e, ...p } : e)));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return setError(t("Please enter a name"));
    if (events.filter((e) => e.kind === "birthday").length > 1)
      return setError(t("Only one birthday per contact"));
    if (events.some((e) => !validDate(e.date)))
      return setError(t("Write dates like 08-15 or 1990-08-15"));
    const days = parseDays(remind);
    if (!days)
      return setError(t("Reminders: whole numbers from 1 to 365, at most 8"));
    const period = every.trim() === "" ? 0 : Number(every);
    if (!Number.isInteger(period) || period < 0 || period > 3650)
      return setError(t("Contact period: whole days from 0 to 3650"));
    const body = {
      name: name.trim(),
      group,
      events: events.map((e) => ({
        ...(e.id ? { id: e.id } : {}),
        kind: e.kind,
        label: e.label.trim(),
        date: e.date.trim(),
      })),
      lastContactOn: lastContact,
      contactEveryDays: period,
      remindDays: days,
      notes,
    };
    try {
      if (c) await update.mutateAsync({ id: c.id, body });
      else await create.mutateAsync(body);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={c ? t("Edit contact") : t("New contact")}
      wide
    >
      <form onSubmit={submit}>
        <div className="contacts-form-row">
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              autoFocus
            />
          </label>
          <label className="xc-field contacts-narrow">
            <span>{t("Group")}</span>
            <select
              className="xc-select"
              value={group}
              onChange={(e) => setGroup(e.target.value as ContactGroup)}
            >
              {GROUPS.map((g) => (
                <option key={g} value={g}>
                  {t(GROUP_LABELS[g])}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="contacts-events">
          <div className="contacts-events-head">
            <strong>{t("Important dates")}</strong>
            <button
              type="button"
              className="xc-btn small"
              onClick={() =>
                setEvents((list) => [
                  ...list,
                  {
                    kind: list.some((e) => e.kind === "birthday")
                      ? "other"
                      : "birthday",
                    label: "",
                    date: "",
                  },
                ])
              }
            >
              <Plus size={14} /> {t("Add date")}
            </button>
          </div>
          {events.length === 0 && (
            <small className="xc-muted">{t("No dates.")}</small>
          )}
          {events.map((e, i) => (
            <div className="contacts-event-row" key={e.id ?? `new-${i}`}>
              <select
                className="xc-select"
                aria-label={`${t("Important dates")} ${i + 1} ${t("Type")}`}
                value={e.kind}
                onChange={(ev) =>
                  patch(i, { kind: ev.target.value as ContactEventKind })
                }
              >
                {EVENT_KINDS.map((k) => (
                  <option key={k} value={k}>
                    {t(EVENT_KIND_LABELS[k])}
                  </option>
                ))}
              </select>
              <input
                className="xc-input"
                aria-label={`${t("Important dates")} ${i + 1} ${t("Date name")}`}
                value={e.label}
                maxLength={50}
                placeholder={KIND_DEFAULT_LABEL[e.kind]}
                onChange={(ev) => patch(i, { label: ev.target.value })}
              />
              <input
                className="xc-input"
                aria-label={`${t("Important dates")} ${i + 1} ${t("Date")}`}
                value={e.date}
                placeholder={t("MM-DD or YYYY-MM-DD")}
                autoComplete="off"
                onChange={(ev) => patch(i, { date: ev.target.value })}
              />
              <button
                type="button"
                className="xc-btn ghost danger small"
                aria-label={`${t("Remove")} ${e.label || t(EVENT_KIND_LABELS[e.kind])}`}
                onClick={() =>
                  setEvents((list) => list.filter((_, j) => j !== i))
                }
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          {events.length > 0 && (
            <small className="xc-muted">
              {t(
                "Write the year to see the age. Leave it out if you do not know it.",
              )}
            </small>
          )}
        </div>

        <div className="contacts-form-row">
          <label className="xc-field">
            <span>{t("Last contact on")}</span>
            <input
              className="xc-input"
              type="date"
              value={lastContact}
              max={today()}
              onChange={(e) => setLastContact(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Stay in touch every (days)")}</span>
            <input
              className="xc-input"
              value={every}
              onChange={(e) => setEvery(e.target.value)}
              inputMode="numeric"
              placeholder="90"
            />
            <small>
              {t(
                "You are reminded once when you have not been in touch for this long.",
              )}
            </small>
          </label>
          <label className="xc-field">
            <span>{t("Remind before (days)")}</span>
            <input
              className="xc-input"
              value={remind}
              onChange={(e) => setRemind(e.target.value)}
              inputMode="numeric"
            />
            <small>
              {t("For example 7, 1. You are also told on the day itself.")}
            </small>
          </label>
        </div>
        <div className="xc-field">
          <span>{t("Contact remarks")}</span>
          <MarkdownEditor
            label={t("Contact notes")}
            value={notes}
            onChange={setNotes}
            minRows={3}
          />
        </div>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button type="submit" className="xc-btn primary" disabled={saving}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
