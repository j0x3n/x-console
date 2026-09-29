import { useState } from "react";
import { ChevronRight, Folder, FolderPlus, HardDrive } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import {
  namesIn,
  useCreateFolder,
  useDriveFolders,
  type ConflictPolicy,
} from "../api";
import { conflictNames } from "../logic";

export type TransferMode = "move" | "copy" | "extract";

const TITLES: Record<TransferMode, string> = {
  move: "Move to",
  copy: "Copy to",
  extract: "Extract to",
};
const SUBMIT: Record<TransferMode, string> = {
  move: "Move here",
  copy: "Copy here",
  extract: "Extract here",
};

/*
 * 选目标文件夹（B31）：移动、复制、解压共用。
 * - 从根目录一层层点进去，可以就地新建文件夹。
 * - 被移动或复制的文件夹自己不能作为目标（服务端也会拒绝）。
 * - 移动、复制前先看目标里有没有重名，有就问：跳过、覆盖、保留两个。
 * - 解压看不到压缩包里有什么，直接在下面选重名时怎么办。
 */
export default function TransferDialog({
  mode,
  names,
  exclude,
  hidden,
  busy,
  onSubmit,
  onClose,
}: {
  mode: TransferMode;
  /** 要移动或复制的条目名字，用来判断重名。解压时是压缩包的名字。 */
  names: string[];
  exclude: number[];
  hidden: boolean;
  busy?: boolean;
  onSubmit: (targetId: number, conflict: ConflictPolicy) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [trail, setTrail] = useState<{ id: number; name: string }[]>([]);
  const [conflicts, setConflicts] = useState<string[] | null>(null);
  const [policy, setPolicy] = useState<ConflictPolicy>("rename");
  const [checking, setChecking] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const current = trail.length ? trail[trail.length - 1].id : null;
  // 隐藏区的根目录下只有隐藏条目；进了隐藏文件夹后，里面都算隐藏。
  const folders = useDriveFolders(current, hidden);
  const createFolder = useCreateFolder();
  const shown = (folders.data ?? []).filter((f) => !exclude.includes(f.id));
  const target = current ?? 0;

  const submit = async () => {
    if (mode === "extract") return onSubmit(target, policy);
    setChecking(true);
    try {
      const taken = conflictNames(names, await namesIn(target, hidden));
      if (taken.length === 0) onSubmit(target, "rename");
      else setConflicts(taken);
    } catch {
      // 查不到就交给服务端按“保留两个”处理。
      onSubmit(target, "rename");
    } finally {
      setChecking(false);
    }
  };

  const addFolder = () => {
    const name = newName.trim();
    if (!name) return;
    createFolder.mutate(
      { name, parentId: current ?? undefined, hidden: hidden || undefined },
      {
        onSuccess: (f) => {
          setCreating(false);
          setNewName("");
          setTrail([...trail, { id: f.id, name: f.name }]);
        },
      },
    );
  };

  if (conflicts)
    return (
      <Dialog
        open
        onClose={onClose}
        title={t("Some names already exist")}
        description={`${t("Target folder has")} ${conflicts.length} ${t("items with the same name")}`}
      >
        <ul className="drive-conflicts">
          {conflicts.slice(0, 5).map((n) => (
            <li key={n}>{n}</li>
          ))}
          {conflicts.length > 5 && (
            <li className="drive-muted">
              {t("and")} {conflicts.length - 5} {t("more")}
            </li>
          )}
        </ul>
        <div className="xc-dialog-actions drive-conflict-actions">
          <button
            type="button"
            className="xc-btn ghost"
            onClick={() => setConflicts(null)}
          >
            {t("Back")}
          </button>
          <span className="xc-spacer" />
          <button
            type="button"
            className="xc-btn"
            disabled={busy}
            onClick={() => onSubmit(target, "skip")}
          >
            {t("Skip these")}
          </button>
          <button
            type="button"
            className="xc-btn"
            disabled={busy}
            title={t("Old files go to the trash")}
            onClick={() => onSubmit(target, "overwrite")}
          >
            {t("Replace")}
          </button>
          <button
            type="button"
            className="xc-btn primary"
            disabled={busy}
            onClick={() => onSubmit(target, "rename")}
          >
            {t("Keep both")}
          </button>
        </div>
      </Dialog>
    );

  return (
    <Dialog
      open
      onClose={onClose}
      title={t(TITLES[mode])}
      description={
        mode === "extract" ? names[0] : `${names.length} ${t("items")}`
      }
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
        ) : shown.length === 0 && !creating ? (
          <p className="drive-muted">{t("No folders here")}</p>
        ) : (
          shown.map((f) => (
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
        {creating && (
          <form
            className="drive-move-new"
            onSubmit={(e) => {
              e.preventDefault();
              addFolder();
            }}
          >
            <FolderPlus size={15} />
            <input
              className="xc-input"
              autoFocus
              value={newName}
              placeholder={t("Folder name")}
              aria-label={t("Folder name")}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.stopPropagation();
                  setCreating(false);
                }
              }}
            />
            <button
              type="submit"
              className="xc-btn small"
              disabled={!newName.trim() || createFolder.isPending}
            >
              {t("Create")}
            </button>
          </form>
        )}
      </div>
      {mode === "extract" && (
        <label className="xc-field drive-conflict-select">
          <span>{t("When a name exists")}</span>
          <select
            className="xc-select"
            value={policy}
            onChange={(e) => setPolicy(e.target.value as ConflictPolicy)}
          >
            <option value="rename">{t("Keep both")}</option>
            <option value="skip">{t("Skip these")}</option>
            <option value="overwrite">{t("Replace")}</option>
          </select>
        </label>
      )}
      <div className="xc-dialog-actions">
        {!creating && (
          <button
            type="button"
            className="xc-btn ghost"
            onClick={() => setCreating(true)}
          >
            <FolderPlus size={14} /> {t("New folder")}
          </button>
        )}
        <span className="xc-spacer" />
        <button type="button" className="xc-btn ghost" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={busy || checking}
          onClick={() => void submit()}
        >
          {t(SUBMIT[mode])}
        </button>
      </div>
    </Dialog>
  );
}
