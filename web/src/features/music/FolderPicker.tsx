import { ChevronRight, Folder, HardDrive } from "lucide-react";
import { useState } from "react";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useDriveFolders } from "../drive/api";

/**
 * 选一个云盘文件夹作音乐目录（B147）。从根目录一层层点进去，
 * 点“选择这个文件夹”确定。根目录本身不能选。
 */
export default function FolderPicker({
  exclude,
  onPick,
  onClose,
}: {
  /** 已经选过的文件夹，不再列出。 */
  exclude: number[];
  onPick: (id: number) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [trail, setTrail] = useState<{ id: number; name: string }[]>([]);
  const current = trail.length ? trail[trail.length - 1].id : null;
  const folders = useDriveFolders(current, false);
  const shown = (folders.data ?? []).filter((f) => !exclude.includes(f.id));

  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Choose a music folder")}
      description={t("Songs in this folder and its subfolders are added.")}
    >
      <nav className="music-trail" aria-label={t("Folder path")}>
        <button
          type="button"
          className="xc-btn small ghost"
          onClick={() => setTrail([])}
        >
          <HardDrive size={13} /> {t("Drive")}
        </button>
        {trail.map((step, i) => (
          <span key={step.id} className="music-trail-step">
            <ChevronRight size={13} />
            <button
              type="button"
              className="xc-btn small ghost"
              onClick={() => setTrail(trail.slice(0, i + 1))}
            >
              {step.name}
            </button>
          </span>
        ))}
      </nav>
      <div className="music-pick-list">
        {folders.isPending && <Loading />}
        {folders.isError && (
          <ErrorState error={folders.error} onRetry={() => folders.refetch()} />
        )}
        {folders.isSuccess && shown.length === 0 && (
          <p className="music-muted">{t("No more folders here")}</p>
        )}
        {shown.map((f) => (
          <button
            key={f.id}
            type="button"
            className="music-pick-row"
            onClick={() => setTrail([...trail, { id: f.id, name: f.name }])}
          >
            <span>
              <Folder size={14} /> {f.name}
            </span>
            <ChevronRight size={14} />
          </button>
        ))}
      </div>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={current == null}
          onClick={() => current != null && onPick(current)}
        >
          {t("Choose this folder")}
        </button>
      </div>
    </Dialog>
  );
}
