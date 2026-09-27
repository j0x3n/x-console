import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  AlarmClock,
  Check,
  Clock,
  ExternalLink,
  Pencil,
  Plus,
  Repeat,
  Trash2,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCompleteReminder,
  useDeleteReminder,
  useReminders,
  useSnoozeReminder,
  type Reminder,
  type ReminderRange,
} from "./api";
import ReminderDialog from "./ReminderDialog";
import { describeRule, formatWhen } from "./rrule";

const tabs: { id: ReminderRange; label: string }[] = [
  { id: "today", label: "Today" },
  { id: "upcoming", label: "Upcoming" },
  { id: "done", label: "Done" },
];

const emptyText: Record<ReminderRange, string> = {
  today: "Nothing to do today",
  upcoming: "No upcoming reminders",
  done: "Nothing completed yet",
};

export default function RemindersPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const tab = (tabs.find((x) => x.id === params.get("tab"))?.id ??
    "today") as ReminderRange;
  const creating = params.get("new") === "1";
  const [editing, setEditing] = useState<Reminder | null>(null);
  const list = useReminders(tab);

  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(params);
    if (value === null) next.delete(key);
    else next.set(key, value);
    setParams(next, { replace: true });
  };

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Reminders")}
        aside={
          <button
            className="xc-btn primary"
            onClick={() => setParam("new", "1")}
          >
            <Plus size={15} /> {t("New reminder")}
          </button>
        }
      />
      <nav className="xc-tabs">
        {tabs.map((item) => (
          <button
            key={item.id}
            className={item.id === tab ? "active" : ""}
            onClick={() =>
              setParam("tab", item.id === "today" ? null : item.id)
            }
          >
            {t(item.label)}
          </button>
        ))}
      </nav>
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 ? (
        <EmptyState title={t(emptyText[tab])} icon={<AlarmClock size={28} />} />
      ) : (
        <div className="xc-card reminders-list">
          {list.data.map((r) => (
            <ReminderRow key={r.id} reminder={r} onEdit={() => setEditing(r)} />
          ))}
        </div>
      )}
      <ReminderDialog
        open={creating || editing !== null}
        reminder={editing}
        onClose={() => {
          setEditing(null);
          if (creating) setParam("new", null);
        }}
      />
    </div>
  );
}

function ReminderRow({
  reminder: r,
  onEdit,
}: {
  reminder: Reminder;
  onEdit: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const complete = useCompleteReminder();
  const snooze = useSnoozeReminder();
  const remove = useDeleteReminder();
  const finished = r.status === "done" || r.status === "ended";
  const onError = (err: unknown) =>
    toast({ message: errorMessage(err), tone: "error" });
  const now = new Date();
  const when =
    r.status === "pending" && r.lastFiredAt
      ? r.lastFiredAt
      : finished
        ? (r.doneAt ?? r.lastFiredAt ?? r.dtstart)
        : (r.dueAt ?? r.dtstart);

  return (
    <div className={`reminders-row${finished ? " is-done" : ""}`}>
      <button
        className="reminders-check-btn"
        title={t("Mark done")}
        aria-label={t("Mark done")}
        disabled={finished || complete.isPending}
        onClick={() =>
          complete.mutate(r.id, {
            onSuccess: () => toast(t("Done")),
            onError,
          })
        }
      >
        {finished && <Check size={13} />}
      </button>
      <div className="reminders-main">
        <div className="reminders-title">
          <span>{r.title}</span>
          {r.status === "pending" && (
            <span className="xc-badge warn">{t("Due")}</span>
          )}
          {r.status === "snoozed" && (
            <span className="xc-badge info">{t("Snoozed")}</span>
          )}
          {!r.enabled && !finished && (
            <span className="xc-badge">{t("Paused")}</span>
          )}
        </div>
        <div className="reminders-meta">
          <span>
            <Clock size={12} /> {formatWhen(when, now, language)}
          </span>
          {r.rrule && (
            <span>
              <Repeat size={12} />{" "}
              {describeRule(r.rrule, new Date(r.dtstart), language)}
            </span>
          )}
          {r.link &&
            (r.link.startsWith("/") ? (
              <Link to={r.link}>
                <ExternalLink size={12} /> {r.link}
              </Link>
            ) : (
              <a href={r.link} target="_blank" rel="noreferrer">
                <ExternalLink size={12} /> {t("Open link")}
              </a>
            ))}
        </div>
        {r.body && <p className="reminders-body">{r.body}</p>}
      </div>
      <div className="reminders-actions">
        {!finished && (
          <button
            className="xc-btn ghost small"
            title={t("Remind me in 10 minutes")}
            disabled={snooze.isPending}
            onClick={() =>
              snooze.mutate(
                { id: r.id, minutes: 10 },
                {
                  onSuccess: () => toast(t("Will remind you in 10 minutes")),
                  onError,
                },
              )
            }
          >
            <AlarmClock size={14} /> 10′
          </button>
        )}
        <button
          className="xc-btn ghost small"
          aria-label={t("Edit")}
          title={t("Edit")}
          onClick={onEdit}
        >
          <Pencil size={14} />
        </button>
        <button
          className="xc-btn ghost small"
          aria-label={t("Delete")}
          title={t("Delete")}
          disabled={remove.isPending}
          onClick={() =>
            confirm(`${t("Delete")} “${r.title}”?`) &&
            remove.mutate(r.id, {
              onSuccess: () => toast(t("Deleted")),
              onError,
            })
          }
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
}
