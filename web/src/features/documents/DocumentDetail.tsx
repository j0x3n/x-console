import { useRef, useState } from "react";
import {
  Archive,
  ArchiveRestore,
  CalendarCheck,
  Download,
  ImagePlus,
  Paperclip,
  Pencil,
  Trash2,
  X,
} from "lucide-react";
import Markdown from "../../components/markdown/Markdown";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { showError } from "./util";
import { contentUrl } from "../drive/api";
import {
  useDeleteDocument,
  useRemoveDocumentFile,
  useUpdateDocument,
  type DocumentItem,
} from "./api";
import DocumentDialog from "./DocumentDialog";
import { daysText, kindName, statusTone } from "./format";
import { PhotoGrid, isPhoto, useAttachFiles } from "./photos";

function longDate(value: string, language: string): string {
  return new Date(`${value}T00:00:00`).toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    { year: "numeric", month: "short", day: "numeric" },
  );
}

/** 一份档案的详情：各项内容、扫描件，以及续期、归档、删除。 */
export default function DocumentDetail({
  document: doc,
  onClose,
}: {
  document: DocumentItem | null;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const update = useUpdateDocument();
  const remove = useDeleteDocument();
  const attach = useAttachFiles();
  const removeFile = useRemoveDocumentFile();
  const fileInput = useRef<HTMLInputElement>(null);
  const photoInput = useRef<HTMLInputElement>(null);
  const [editing, setEditing] = useState(false);
  const [renewing, setRenewing] = useState(false);
  const [renewDate, setRenewDate] = useState("");
  const [uploading, setUploading] = useState(false);
  if (!doc) return null;

  const archive = () =>
    update.mutate(
      { id: doc.id, body: { archived: !doc.archived } },
      {
        onSuccess: () => {
          toast(doc.archived ? t("Unarchived") : t("Archived"));
          onClose();
        },
        onError: showError,
      },
    );
  const renew = () => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(renewDate)) return;
    update.mutate(
      { id: doc.id, body: { expiresOn: renewDate } },
      {
        onSuccess: () => {
          toast(t("Document renewed"));
          setRenewing(false);
        },
        onError: showError,
      },
    );
  };
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete this document?")} ${doc.name}`,
      description: t("The record is deleted. The files in the drive stay."),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(doc.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        onClose();
      },
      onError: showError,
    });
  };
  const upload = async (files: FileList | null) => {
    if (!files?.length) return;
    setUploading(true);
    try {
      await attach(doc.id, Array.from(files));
    } catch (err) {
      showError(err);
    } finally {
      setUploading(false);
      if (fileInput.current) fileInput.current.value = "";
      if (photoInput.current) photoInput.current.value = "";
    }
  };

  const removeOne = (driveId: number) =>
    removeFile.mutate(
      { id: doc.id, driveId },
      {
        onSuccess: () =>
          toast(t("Removed from this document. The file stays in the drive.")),
        onError: showError,
      },
    );

  const facts: Array<[string, string | null]> = [
    [t("Type"), kindName(t, doc.kind)],
    [t("Holder"), doc.holder || null],
    [t("Document number"), doc.number || null],
    [
      t("Issued or bought on"),
      doc.issuedOn ? longDate(doc.issuedOn, language) : null,
    ],
    [t("Expires on"), doc.expiresOn ? longDate(doc.expiresOn, language) : null],
    [
      t("Price"),
      doc.price != null ? `${doc.price} ${doc.currency}`.trim() : null,
    ],
    [t("Serial number"), doc.serial || null],
    [
      t("Reminders"),
      doc.remindDays.length
        ? `${doc.remindDays.join(", ")} ${t("days before")}`
        : t("No reminders"),
    ],
  ];

  return (
    <>
      <Dialog open={!editing} onClose={onClose} title={doc.name} wide>
        <p className="documents-status">
          {doc.archived ? (
            <span className="xc-badge">{t("Archived")}</span>
          ) : (
            <span
              className={`xc-badge ${statusTone(doc.status, doc.daysLeft)}`}
            >
              {daysText(t, doc.daysLeft)}
            </span>
          )}
        </p>
        <div className="documents-facts">
          {facts
            .filter(([, value]) => value)
            .map(([label, value]) => (
              <div key={label}>
                <small>{label}</small>
                <strong>{value}</strong>
              </div>
            ))}
        </div>
        {doc.notes && (
          <div className="documents-notes">
            <Markdown source={doc.notes} />
          </div>
        )}
        <div className="documents-files-head">
          <strong>{t("Photos and files")}</strong>
          <span className="documents-files-actions">
            <button
              className="xc-btn small"
              disabled={uploading}
              onClick={() => photoInput.current?.click()}
            >
              <ImagePlus size={14} />{" "}
              {uploading ? t("Uploading…") : t("Add photos")}
            </button>
            <button
              className="xc-btn small"
              disabled={uploading}
              onClick={() => fileInput.current?.click()}
            >
              <Paperclip size={14} /> {t("Upload scan")}
            </button>
          </span>
          <input
            ref={photoInput}
            type="file"
            accept="image/*"
            multiple
            hidden
            data-testid="photo-input"
            onChange={(e) => void upload(e.target.files)}
          />
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            data-testid="scan-input"
            onChange={(e) => void upload(e.target.files)}
          />
        </div>
        {doc.files.length === 0 ? (
          <p className="xc-muted documents-hint">{t("No files yet")}</p>
        ) : null}
        <PhotoGrid
          photos={doc.files.filter(isPhoto)}
          onRemove={(f) => removeOne(f.driveId)}
        />
        {doc.files.some((f) => !isPhoto(f)) && (
          <ul className="documents-files">
            {doc.files
              .filter((f) => !isPhoto(f))
              .map((f) => (
                <li key={f.driveId}>
                  <a href={contentUrl(f.driveId)} download>
                    <Download size={13} /> {f.name}
                  </a>
                  <button
                    className="xc-btn ghost small"
                    title={t("Remove from this document")}
                    aria-label={`${t("Remove from this document")} ${f.name}`}
                    onClick={() => removeOne(f.driveId)}
                  >
                    <X size={13} />
                  </button>
                </li>
              ))}
          </ul>
        )}
        <p className="xc-muted documents-hint">
          {t(
            "Files are kept in the drive folder “Documents”. Removing one here does not delete it from the drive.",
          )}{" "}
          {t(
            "Files are saved as hidden files while hidden content is unlocked.",
          )}
        </p>
        {renewing && (
          <div className="documents-renew">
            <label className="xc-field">
              <span>{t("New expiry date")}</span>
              <input
                className="xc-input"
                type="date"
                value={renewDate}
                autoFocus
                onChange={(e) => setRenewDate(e.target.value)}
              />
            </label>
            <button
              className="xc-btn primary"
              disabled={!renewDate || update.isPending}
              onClick={renew}
            >
              {t("Save")}
            </button>
            <button className="xc-btn" onClick={() => setRenewing(false)}>
              {t("Cancel")}
            </button>
          </div>
        )}
        <div className="xc-dialog-actions documents-actions">
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
            {doc.archived ? (
              <ArchiveRestore size={14} />
            ) : (
              <Archive size={14} />
            )}{" "}
            {doc.archived ? t("Unarchive") : t("Archive")}
          </button>
          <span className="xc-spacer" />
          <button className="xc-btn" onClick={() => setEditing(true)}>
            <Pencil size={14} /> {t("Edit")}
          </button>
          {!doc.archived && (
            <button
              className="xc-btn primary"
              onClick={() => setRenewing(true)}
            >
              <CalendarCheck size={14} /> {t("Renew")}
            </button>
          )}
        </div>
      </Dialog>
      <DocumentDialog
        open={editing}
        onClose={() => setEditing(false)}
        document={doc}
      />
    </>
  );
}
