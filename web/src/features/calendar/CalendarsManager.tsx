import { useEffect, useState, type CSSProperties } from "react";
import { useSearchParams } from "react-router";
import { CalendarDays, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useCalendars,
  useCreateCalendar,
  useDeleteCalendar,
  useSyncCalendar,
  useUpdateCalendar,
  type Calendar,
  type CalendarPatch,
} from "./api";
import { confirmAction } from "../../components/ui/ConfirmDialog";

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 日历管理：添加 ICS 订阅或 CalDAV 账号，看同步状态。 */
export default function CalendarsManager() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const creating = !!params.get("new");
  const presetKind = params.get("new") === "local" ? "local" : undefined;
  const [editing, setEditing] = useState<Calendar | null>(null);
  const list = useCalendars();

  const closeDialog = () => {
    setEditing(null);
    if (creating) {
      const next = new URLSearchParams(params);
      next.delete("new");
      setParams(next, { replace: true });
    }
  };

  return (
    <div className="xc-stack">
      <div className="xc-row">
        <span className="xc-muted calendar-note">
          {t(
            "Local and CalDAV calendars can be edited here. ICS links are read only. Remote calendars sync every 15 minutes.",
          )}
        </span>
        <span className="xc-spacer" />
        <button
          className="xc-btn primary"
          onClick={() => setParams({ new: "1" }, { replace: true })}
        >
          <Plus size={15} /> {t("Add calendar")}
        </button>
      </div>
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 ? (
        <EmptyState
          title={t("No calendars yet")}
          icon={<CalendarDays size={28} />}
        >
          <span>{t("Add an ICS link or a CalDAV account.")}</span>
        </EmptyState>
      ) : (
        <div className="xc-card calendar-list">
          {list.data.map((c) => (
            <CalendarRow key={c.id} calendar={c} onEdit={() => setEditing(c)} />
          ))}
        </div>
      )}
      <CalendarDialog
        open={creating || editing !== null}
        calendar={editing}
        presetKind={presetKind}
        onClose={closeDialog}
      />
    </div>
  );
}

function CalendarRow({
  calendar: c,
  onEdit,
}: {
  calendar: Calendar;
  onEdit: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const sync = useSyncCalendar();
  const update = useUpdateCalendar();
  const remove = useDeleteCalendar();
  return (
    <div className={`calendar-item${c.enabled ? "" : " is-off"}`}>
      <span
        className="calendar-dot"
        style={{ "--ev": c.color || undefined } as CSSProperties}
      />
      <div className="calendar-item-main">
        <div className="calendar-item-title">
          <strong>{c.name}</strong>
          <span className="xc-badge">
            {c.kind === "ics"
              ? "ICS"
              : c.kind === "caldav"
                ? "CalDAV"
                : t("Local")}
          </span>
          {c.kind !== "ics" && (
            <span className="xc-badge ok">{t("Editable")}</span>
          )}
          {!c.enabled && <span className="xc-badge warn">{t("Hidden")}</span>}
        </div>
        {c.url && <div className="calendar-item-url xc-mono">{c.url}</div>}
        <div className="calendar-item-meta">
          {c.kind === "local" ? (
            <span>
              {c.eventCount} {t("events")}
            </span>
          ) : c.lastError ? (
            <span className="calendar-error">{c.lastError}</span>
          ) : c.lastSyncedAt ? (
            <span>
              {t("Synced")} {relativeTime(c.lastSyncedAt, language)} ·{" "}
              {c.eventCount} {t("events")}
            </span>
          ) : (
            <span>{t("Not synced yet")}</span>
          )}
        </div>
      </div>
      <div className="calendar-item-actions">
        <label className="calendar-toggle" title={t("Show in calendar")}>
          <input
            type="checkbox"
            checked={c.enabled}
            disabled={update.isPending}
            onChange={(e) =>
              update.mutate(
                { id: c.id, body: { enabled: e.target.checked } },
                { onError },
              )
            }
          />
          <span>{t("Show")}</span>
        </label>
        {c.kind !== "local" && (
          <button
            className="xc-btn ghost small"
            title={t("Sync now")}
            aria-label={t("Sync now")}
            disabled={sync.isPending || !c.enabled}
            onClick={() =>
              sync.mutate(c.id, {
                onSuccess: (res) =>
                  res.lastError
                    ? toast({ message: res.lastError, tone: "error" })
                    : toast(t("Synced")),
                onError,
              })
            }
          >
            <RefreshCw
              size={14}
              className={sync.isPending ? "calendar-spin" : ""}
            />
          </button>
        )}
        <button
          className="xc-btn ghost small"
          title={t("Edit")}
          aria-label={t("Edit")}
          onClick={onEdit}
        >
          <Pencil size={14} />
        </button>
        <button
          className="xc-btn ghost small"
          title={t("Delete")}
          aria-label={t("Delete")}
          disabled={remove.isPending}
          onClick={async () => {
            if (
              !(await confirmAction({
                title: t("Delete this calendar and its synced events?"),
              }))
            )
              return;
            remove.mutate(c.id, {
              onSuccess: () => toast(t("Deleted")),
              onError,
            });
          }}
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
}

interface FormState {
  name: string;
  kind: "ics" | "caldav" | "local";
  url: string;
  username: string;
  password: string;
  color: string;
}

const emptyForm: FormState = {
  name: "",
  kind: "local",
  url: "",
  username: "",
  password: "",
  color: "",
};

function CalendarDialog({
  open,
  calendar,
  presetKind,
  onClose,
}: {
  open: boolean;
  calendar: Calendar | null;
  presetKind?: FormState["kind"];
  onClose: () => void;
}) {
  const t = useT();
  const create = useCreateCalendar();
  const update = useUpdateCalendar();
  const [form, setForm] = useState<FormState>(emptyForm);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setForm(
      calendar
        ? {
            name: calendar.name,
            kind: calendar.kind,
            url: calendar.url,
            username: calendar.username,
            password: "",
            color: calendar.color,
          }
        : { ...emptyForm, kind: presetKind ?? emptyForm.kind },
    );
  }, [open, calendar, presetKind]);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((f) => ({ ...f, [key]: value }));

  const submit = () => {
    if (!form.name.trim()) return setError(t("Name is required"));
    const local = form.kind === "local";
    if (!local && !form.url.trim()) return setError(t("Address is required"));
    const done = () => {
      toast(local ? t("Saved") : t("Saved. Syncing now."));
      onClose();
    };
    const fail = (err: unknown) => setError(errorMessage(err));
    if (calendar) {
      const body: CalendarPatch = {
        name: form.name,
        url: form.url,
        username: form.username,
        color: form.color,
      };
      if (form.password) body.password = form.password;
      update.mutate(
        { id: calendar.id, body },
        { onSuccess: done, onError: fail },
      );
    } else {
      create.mutate(
        {
          name: form.name,
          kind: form.kind,
          url: form.url,
          username: form.username || undefined,
          password: form.password || undefined,
          color: form.color || undefined,
        },
        { onSuccess: done, onError: fail },
      );
    }
  };

  const pending = create.isPending || update.isPending;
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={calendar ? t("Edit calendar") : t("Add calendar")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={pending}
            onClick={submit}
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            autoFocus
          />
        </label>
        {!calendar && (
          <label className="xc-field">
            <span>{t("Type")}</span>
            <select
              className="xc-select"
              value={form.kind}
              onChange={(e) => set("kind", e.target.value as FormState["kind"])}
            >
              <option value="local">{t("Local calendar")}</option>
              <option value="caldav">{t("CalDAV, such as iCloud")}</option>
              <option value="ics">{t("ICS subscription link")}</option>
            </select>
          </label>
        )}
        {form.kind === "local" && (
          <p className="calendar-kind-hint">
            {t("Saved on your own server. Add and edit events here.")}
          </p>
        )}
        {form.kind !== "local" && (
          <>
            <label className="xc-field">
              <span>{t("Address")}</span>
              <input
                className="xc-input"
                value={form.url}
                inputMode="url"
                placeholder={
                  form.kind === "ics"
                    ? "https://…/basic.ics"
                    : "https://caldav.example.com/"
                }
                onChange={(e) => set("url", e.target.value)}
              />
              <small>
                {form.kind === "ics"
                  ? t(
                      "A webcal:// link works too. For Google Calendar, use the secret address in iCal format from its settings.",
                    )
                  : t(
                      "For iCloud, use https://caldav.icloud.com, your Apple ID and an app-specific password. Events you add here are written back.",
                    )}
              </small>
            </label>
            <div className="calendar-form-row">
              <label className="xc-field">
                <span>
                  {t("Username")}{" "}
                  <small className="xc-muted">{t("optional")}</small>
                </span>
                <input
                  className="xc-input"
                  value={form.username}
                  autoComplete="off"
                  onChange={(e) => set("username", e.target.value)}
                />
              </label>
              <label className="xc-field">
                <span>
                  {t("Password")}{" "}
                  <small className="xc-muted">{t("optional")}</small>
                </span>
                <input
                  className="xc-input"
                  type="password"
                  value={form.password}
                  autoComplete="new-password"
                  placeholder={
                    calendar?.hasPassword ? t("Leave empty to keep it") : ""
                  }
                  onChange={(e) => set("password", e.target.value)}
                />
              </label>
            </div>
          </>
        )}
        <label className="xc-field">
          <span>{t("Color")}</span>
          <div className="xc-row">
            <input
              type="color"
              className="calendar-color"
              value={form.color || "#cc7752"}
              onChange={(e) => set("color", e.target.value)}
            />
            {form.color && (
              <button
                type="button"
                className="xc-btn ghost small"
                onClick={() => set("color", "")}
              >
                {t("Default color")}
              </button>
            )}
          </div>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
