import { useState, type FormEvent } from "react";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { nameError } from "../logic";

/** 新建文件夹和改名共用。 */
export default function NameDialog({
  title,
  initial,
  submitLabel,
  busy,
  onSubmit,
  onClose,
}: {
  title: string;
  initial: string;
  submitLabel: string;
  busy?: boolean;
  onSubmit: (name: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [name, setName] = useState(initial);
  const [error, setError] = useState("");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const problem = nameError(name);
    setError(problem);
    if (!problem) onSubmit(name.trim());
  };
  return (
    <Dialog open onClose={onClose} title={title}>
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            autoFocus
            value={name}
            onFocus={(e) => {
              // 改名时先选中扩展名前面的部分。
              const dot = initial.lastIndexOf(".");
              e.target.setSelectionRange(0, dot > 0 ? dot : initial.length);
            }}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={busy}>
            {submitLabel}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
