import { FolderOpen, FolderPlus } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { archiveBase } from "../logic";

/**
 * 解压（B31）：默认解压到同名的新文件夹，也可以自己选位置。
 * 选位置时由外面打开 TransferDialog。
 */
export default function ExtractDialog({
  name,
  busy,
  onHere,
  onPick,
  onClose,
}: {
  name: string;
  busy?: boolean;
  onHere: () => void;
  onPick: () => void;
  onClose: () => void;
}) {
  const t = useT();
  return (
    <Dialog open onClose={onClose} title={t("Extract")} description={name}>
      <div className="drive-extract-options">
        <button
          type="button"
          className="drive-extract-option"
          disabled={busy}
          onClick={onHere}
        >
          <FolderPlus size={18} />
          <span>
            <strong>
              {t("Extract to new folder")}“{archiveBase(name)}”
            </strong>
            <small>
              {t("Next to the archive. Adds (1) if the name is taken.")}
            </small>
          </span>
        </button>
        <button
          type="button"
          className="drive-extract-option"
          disabled={busy}
          onClick={onPick}
        >
          <FolderOpen size={18} />
          <span>
            <strong>{t("Choose a folder…")}</strong>
            <small>
              {t("Put the files straight into an existing folder.")}
            </small>
          </span>
        </button>
      </div>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn ghost" onClick={onClose}>
          {t("Cancel")}
        </button>
      </div>
    </Dialog>
  );
}
