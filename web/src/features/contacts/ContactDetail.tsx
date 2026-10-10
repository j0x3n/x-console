import { useState } from "react";
import { Archive, ArchiveRestore, Pencil, Phone, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDeleteContact,
  useTouchContact,
  useUpdateContact,
  type ContactItem,
} from "./api";
import ContactDialog from "./ContactDialog";
import {
  EVENT_KIND_LABELS,
  GROUP_LABELS,
  eventText,
  formatDays,
  sinceText,
  statusText,
  statusTone,
} from "./format";

const showError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 一个联系人的详情：重要日期、联系情况、备注，以及刚联系过、编辑、归档、删除。 */
export default function ContactDetail({
  contact: c,
  onClose,
}: {
  contact: ContactItem | null;
  onClose: () => void;
}) {
  const t = useT();
  const update = useUpdateContact();
  const touch = useTouchContact();
  const remove = useDeleteContact();
  const [editing, setEditing] = useState(false);
  if (!c) return null;

  const archive = () =>
    update.mutate(
      { id: c.id, body: { archived: !c.archived } },
      {
        onSuccess: () => {
          toast(c.archived ? t("Unarchived") : t("Archived"));
          onClose();
        },
        onError: showError,
      },
    );
  const onTouch = () =>
    touch.mutate(
      { id: c.id },
      { onSuccess: () => toast(t("Contact recorded")), onError: showError },
    );
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete this contact?")} ${c.name}`,
      description: t(
        "The contact and its dates are deleted. This can not be undone.",
      ),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(c.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        onClose();
      },
      onError: showError,
    });
  };

  return (
    <>
      <Dialog open={!editing} onClose={onClose} title={c.name} wide>
        <p className="contacts-status">
          <span className="xc-badge">{t(GROUP_LABELS[c.group])}</span>{" "}
          {c.archived ? (
            <span className="xc-badge">{t("Archived")}</span>
          ) : (
            <span className={`xc-badge ${statusTone(c.status)}`}>
              {statusText(t, c)}
            </span>
          )}
        </p>
        <div className="contacts-facts">
          <div>
            <small>{t("Last contact")}</small>
            <strong>
              {c.lastContactOn
                ? `${c.lastContactOn} · ${sinceText(t, c.sinceContact)}`
                : t("No contact recorded")}
            </strong>
          </div>
          <div>
            <small>{t("Contact period")}</small>
            <strong>
              {c.contactEveryDays
                ? `${c.contactEveryDays} ${t("days")}`
                : t("No reminders")}
            </strong>
          </div>
          <div>
            <small>{t("Reminders")}</small>
            <strong>
              {c.remindDays.length
                ? `${formatDays(c.remindDays)} ${t("days before")}`
                : t("No reminders")}
            </strong>
          </div>
        </div>
        {c.events.length > 0 && (
          <ul className="contacts-event-list" aria-label={t("Important dates")}>
            {[...c.events]
              .sort((a, b) => a.nextIn - b.nextIn)
              .map((e) => (
                <li key={e.id}>
                  <strong>{e.label || t(EVENT_KIND_LABELS[e.kind])}</strong>
                  <span>{e.date}</span>
                  <small>
                    {t("Next time")} {e.nextOn} · {eventText(t, e)}
                  </small>
                </li>
              ))}
          </ul>
        )}
        {c.notes && (
          <div className="contacts-notes">
            <Markdown source={c.notes} />
          </div>
        )}
        <div className="xc-dialog-actions contacts-actions">
          <button
            className="xc-btn ghost danger"
            disabled={remove.isPending}
            onClick={() => void onDelete()}
          >
            <Trash2 size={14} /> {t("Delete")}
          </button>
          <button
            className="xc-btn ghost"
            disabled={update.isPending}
            onClick={archive}
          >
            {c.archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}{" "}
            {c.archived ? t("Unarchive") : t("Archive")}
          </button>
          <span className="xc-spacer" />
          <button className="xc-btn" onClick={() => setEditing(true)}>
            <Pencil size={14} /> {t("Edit")}
          </button>
          {!c.archived && (
            <button
              className="xc-btn primary"
              disabled={touch.isPending}
              onClick={onTouch}
            >
              <Phone size={14} /> {t("Just contacted")}
            </button>
          )}
        </div>
      </Dialog>
      <ContactDialog
        open={editing}
        onClose={() => setEditing(false)}
        contact={c}
      />
    </>
  );
}
