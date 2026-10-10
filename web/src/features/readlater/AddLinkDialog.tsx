import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useAddLink } from "./api";
import "./i18n";

/** 手动加一个链接。后台去抓网页，这里不等。 */
export default function AddLinkDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const add = useAddLink();
  const [url, setUrl] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setUrl("");
    setNote("");
    setError("");
  }, [open]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!url.trim()) return setError(t("Please enter a web address"));
    try {
      const res = await add.mutateAsync({
        url: url.trim(),
        note: note.trim(),
      });
      toast(
        res.duplicate
          ? t("This link was saved before")
          : t("Saved. Fetching the page…"),
      );
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("Add link")}>
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Web address")}</span>
          <input
            className="xc-input"
            type="url"
            inputMode="url"
            autoFocus
            placeholder="https://"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Add a note (optional)")}</span>
          <textarea
            className="xc-input"
            rows={3}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={add.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
