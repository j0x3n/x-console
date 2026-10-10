import { useState } from "react";
import {
  Archive,
  ArchiveRestore,
  Pencil,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDeleteCredential,
  useRotateCredential,
  useUpdateCredential,
  type CredentialItem,
} from "./api";
import CredentialDialog from "./CredentialDialog";
import {
  KIND_LABELS,
  dueText,
  plusDays,
  statusTone,
  suggestExpiry,
} from "./format";

function longDate(value: string, language: string): string {
  return new Date(`${value}T00:00:00`).toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    { year: "numeric", month: "short", day: "numeric" },
  );
}

const showError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 一条记录的详情：各项内容，以及更换、编辑、归档、删除。 */
export default function CredentialDetail({
  credential: c,
  onClose,
  onSearch,
}: {
  credential: CredentialItem | null;
  onClose: () => void;
  /** 点“用在哪里”里的一项：回到列表并按它搜索 */
  onSearch: (text: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const update = useUpdateCredential();
  const rotate = useRotateCredential();
  const remove = useDeleteCredential();
  const [editing, setEditing] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [rotatedOn, setRotatedOn] = useState("");
  const [expiresOn, setExpiresOn] = useState("");
  const [hint, setHint] = useState("");
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
  const startRotate = () => {
    const today = plusDays(0);
    setRotatedOn(today);
    setExpiresOn(suggestExpiry(c, today));
    setHint(c.hint);
    setRotating(true);
  };
  const saveRotate = () =>
    rotate.mutate(
      {
        id: c.id,
        body: {
          rotatedOn: rotatedOn || undefined,
          ...(expiresOn ? { expiresOn } : {}),
          hint,
        },
      },
      {
        onSuccess: () => {
          toast(t("Rotation recorded"));
          setRotating(false);
        },
        onError: showError,
      },
    );
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete this credential?")} ${c.name}`,
      description: t(
        "Only the record is deleted. Nothing changes on the platform.",
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

  const date = (v: string) => (v ? longDate(v, language) : null);
  const facts: Array<[string, string | null]> = [
    [t("Type"), t(KIND_LABELS[c.kind])],
    [t("Platform"), c.platform || null],
    [t("Account"), c.account || null],
    [t("Permissions"), c.scopes || null],
    [t("Last characters"), c.hint ? `…${c.hint}` : null],
    [t("Created on"), date(c.createdOn)],
    [t("Last rotated on"), date(c.rotatedOn)],
    [t("Expires on"), date(c.expiresOn)],
    [
      t("Rotate every (days)"),
      c.rotateEveryDays ? `${c.rotateEveryDays} ${t("days")}` : null,
    ],
    [
      t("Reminders"),
      c.remindDays.length
        ? `${c.remindDays.join(", ")} ${t("days before")}`
        : t("No reminders"),
    ],
  ];

  return (
    <>
      <Dialog open={!editing} onClose={onClose} title={c.name} wide>
        <p className="credentials-status">
          {c.archived ? (
            <span className="xc-badge">{t("Archived")}</span>
          ) : (
            <span className={`xc-badge ${statusTone(c.status)}`}>
              {dueText(t, c)}
            </span>
          )}
        </p>
        <div className="credentials-facts">
          {facts
            .filter(([, value]) => value)
            .map(([label, value]) => (
              <div key={label}>
                <small>{label}</small>
                <strong>{value}</strong>
              </div>
            ))}
        </div>
        {c.usedBy.length > 0 && (
          <div className="credentials-used">
            <small>{t("Used on")}</small>
            <div className="credentials-chips">
              {c.usedBy.map((place) => (
                <button
                  key={place}
                  type="button"
                  className="credentials-chip"
                  title={t("Find everything used here")}
                  onClick={() => onSearch(place)}
                >
                  {place}
                </button>
              ))}
            </div>
          </div>
        )}
        {c.notes && (
          <div className="credentials-notes">
            <Markdown source={c.notes} />
          </div>
        )}
        {rotating && (
          <div className="credentials-rotate">
            <label className="xc-field">
              <span>{t("Rotated on")}</span>
              <input
                className="xc-input"
                type="date"
                value={rotatedOn}
                max={plusDays(0)}
                onChange={(e) => setRotatedOn(e.target.value)}
              />
            </label>
            <label className="xc-field">
              <span>{t("New expiry date")}</span>
              <input
                className="xc-input"
                type="date"
                value={expiresOn}
                onChange={(e) => setExpiresOn(e.target.value)}
              />
            </label>
            <label className="xc-field credentials-narrow">
              <span>{t("Last characters")}</span>
              <input
                className="xc-input"
                value={hint}
                maxLength={16}
                autoComplete="off"
                onChange={(e) => setHint(e.target.value)}
              />
            </label>
            <button
              className="xc-btn primary"
              disabled={rotate.isPending}
              onClick={saveRotate}
            >
              {t("Save")}
            </button>
            <button className="xc-btn" onClick={() => setRotating(false)}>
              {t("Cancel")}
            </button>
          </div>
        )}
        <div className="xc-dialog-actions credentials-actions">
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
          {!c.archived && !rotating && (
            <button className="xc-btn primary" onClick={startRotate}>
              <RefreshCw size={14} /> {t("Rotated")}
            </button>
          )}
        </div>
      </Dialog>
      <CredentialDialog
        open={editing}
        onClose={() => setEditing(false)}
        credential={c}
      />
    </>
  );
}
