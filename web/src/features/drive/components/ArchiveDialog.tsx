import { useState } from "react";
import Dialog from "../../../components/ui/Dialog";
import { Segmented } from "../../../components/ui/Toolbar";
import { useT } from "../../../contexts/LanguageContext";

export type ArchiveFormat = "zip" | "tar.gz";

/** 压缩（B31）：起个名字，选 zip 或 tar.gz。压缩包放在当前文件夹。 */
export default function ArchiveDialog({
  count,
  initialName,
  busy,
  onSubmit,
  onClose,
}: {
  count: number;
  initialName: string;
  busy?: boolean;
  onSubmit: (name: string, format: ArchiveFormat) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [name, setName] = useState(initialName);
  const [format, setFormat] = useState<ArchiveFormat>("zip");
  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Compress")}
      description={`${count} ${t("items")}`}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) onSubmit(name.trim(), format);
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <span className="drive-archive-name">
            <input
              className="xc-input"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <small className="drive-muted">.{format}</small>
          </span>
        </label>
        <div className="xc-field">
          <span>{t("Format")}</span>
          <Segmented
            label={t("Format")}
            value={format}
            onChange={setFormat}
            options={[
              { value: "zip", label: "zip" },
              { value: "tar.gz", label: "tar.gz" },
            ]}
          />
          <small>
            {t("zip opens on every system. tar.gz is common on Linux.")}
          </small>
        </div>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={busy || !name.trim()}
          >
            {t("Compress")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
