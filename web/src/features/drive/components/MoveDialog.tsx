import { useState } from "react";
import { ChevronRight, Folder, HardDrive } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { useDriveFolders } from "../api";

/*
 * 选目标文件夹：从根目录一层层点进去，点“移到这里”。
 * 被移动的文件夹自己不能作为目标（服务端也会拒绝移到自己的子目录里）。
 */
export default function MoveDialog({
  count,
  exclude,
  hidden,
  busy,
  onMove,
  onClose,
}: {
  count: number;
  exclude: number[];
  hidden: boolean;
  busy?: boolean;
  onMove: (parentId: number) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [trail, setTrail] = useState<{ id: number; name: string }[]>([]);
  const current = trail.length ? trail[trail.length - 1].id : null;
  // 隐藏区的根目录下只有隐藏条目；进了隐藏文件夹后，里面都算隐藏。
  const folders = useDriveFolders(current, hidden);
  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Move to")}
      description={`${count} ${t("items")}`}
    >
      <nav className="drive-crumbs small" aria-label={t("Path")}>
        <button type="button" onClick={() => setTrail([])}>
          <HardDrive size={13} /> {t("Drive")}
        </button>
        {trail.map((f, i) => (
          <span key={f.id}>
            <ChevronRight size={12} />
            <button
              type="button"
              onClick={() => setTrail(trail.slice(0, i + 1))}
            >
              {f.name}
            </button>
          </span>
        ))}
      </nav>
      <div className="drive-move-list">
        {folders.isPending ? (
          <Loading />
        ) : folders.isError ? (
          <ErrorState error={folders.error} onRetry={() => folders.refetch()} />
        ) : folders.data.filter((f) => !exclude.includes(f.id)).length === 0 ? (
          <p className="drive-muted">{t("No folders here")}</p>
        ) : (
          folders.data
            .filter((f) => !exclude.includes(f.id))
            .map((f) => (
              <button
                key={f.id}
                type="button"
                onClick={() => setTrail([...trail, { id: f.id, name: f.name }])}
              >
                <Folder size={15} />
                <span>{f.name}</span>
                <ChevronRight size={14} />
              </button>
            ))
        )}
      </div>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn ghost" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={busy}
          onClick={() => onMove(current ?? 0)}
        >
          {t("Move here")}
        </button>
      </div>
    </Dialog>
  );
}
