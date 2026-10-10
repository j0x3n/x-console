import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ArrowDown,
  Cloud,
  Copy,
  FileArchive,
  ArrowUp,
  ChevronRight,
  Download,
  EyeOff,
  Files,
  FolderInput,
  FileText,
  FolderPlus,
  HardDrive,
  LayoutGrid,
  List,
  RotateCcw,
  SquarePen,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { SearchBox, Segmented } from "../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatBytes, relativeTime } from "../../lib/time";
import { useVaultUnlocked } from "../vault/api";
import {
  contentUrl,
  isNotLive,
  thumbnailUrl,
  useArchive,
  useBatchLive,
  useBatchMove,
  useCopyItems,
  useCreateFolder,
  useDeleteForever,
  useExtract,
  zipUrl,
  useDriveItem,
  useDriveItems,
  useDriveUsage,
  useMoveItems,
  useRestoreItems,
  useS3Status,
  useTrashItems,
  useUpdateItem,
  type DriveItem,
  type DriveScope,
  type S3Status,
} from "./api";
import FileIcon from "./components/FileIcon";
import ItemMenu, { itemActions, type ItemAction } from "./components/ItemMenu";
import ArchiveDialog from "./components/ArchiveDialog";
import ExtractDialog from "./components/ExtractDialog";
import NameDialog from "./components/NameDialog";
import ShareDialog from "./components/ShareDialog";
import SharesView from "./components/SharesView";
import RemoteView from "./components/RemoteView";
import { useRemoteDrives } from "./remote";
import TaskPanel from "./components/TaskPanel";
import TransferDialog from "./components/TransferDialog";
import VersionsDialog from "./components/VersionsDialog";
import SyncIcon from "./components/SyncIcon";
import UploadPanel from "./components/UploadPanel";
import {
  archiveName,
  daysLeftInTrash,
  fileKind,
  nextSort,
  sortItems,
  type Sort,
  type SortKey,
} from "./logic";
import { useUploads } from "./upload";
import FileViewer from "./viewer/FileViewer";
import { canEdit } from "./viewer/kind";
import { confirmAction } from "../../components/ui/ConfirmDialog";

type Layout = "list" | "grid";
const LAYOUT_KEY = "xc.drive.layout";

function readLayout(): Layout {
  try {
    return localStorage.getItem(LAYOUT_KEY) === "grid" ? "grid" : "list";
  } catch {
    return "list";
  }
}

type DialogState =
  | { kind: "new-folder" }
  | { kind: "rename"; item: DriveItem }
  | { kind: "move" | "copy" | "compress"; ids: number[]; names: string[] }
  | { kind: "extract" | "extract-pick"; item: DriveItem }
  | { kind: "share" | "versions"; item: DriveItem }
  | { kind: "view"; id: number; edit: boolean }
  | null;

export default function DrivePage() {
  const t = useT();
  const usage = useDriveUsage();
  if (usage.isError && isNotLive(usage.error))
    return (
      <div className="xc-page">
        <PageHeading title={t("Drive")} />
        <EmptyState title="云盘还没上线" icon={<HardDrive size={28} />}>
          <span className="drive-muted">
            界面已经做好，服务端接口还在开发。
          </span>
        </EmptyState>
      </div>
    );
  return <DriveBrowser />;
}

function DriveBrowser() {
  const t = useT();
  const language = useLanguage();
  const vaultUnlocked = useVaultUnlocked();
  const [search, setSearch] = useSearchParams();
  const folderParam = Number(search.get("folder"));
  const folder = folderParam > 0 ? folderParam : null;
  const q = search.get("q") ?? "";
  const tab = search.get("view");
  const trash = tab === "trash";
  // 分享管理（B31）是单独的一个标签，不看文件列表。
  const sharesTab = tab === "shares";
  // 备份设置里绑定的网盘（B68），每个一个标签，只能浏览和下载。
  const remotes = useRemoteDrives();
  const remoteDrive =
    tab === "remote"
      ? remotes.data?.items.find((d) => String(d.id) === search.get("remote"))
      : undefined;
  const remoteRef = search.get("ref") ?? "";
  // 分享和网盘标签不看本地文件列表，上传、搜索这些按钮都不显示。
  const special = sharesTab || !!remoteDrive;
  // 隐藏区只在解锁后出现。锁定后地址栏还带着 view=hidden 时当作普通文件。
  const hidden = tab === "hidden" && vaultUnlocked;
  const scope: DriveScope = {
    folder: trash ? null : folder,
    q,
    trash,
    hidden,
  };

  const items = useDriveItems(scope);
  const current = useDriveItem(trash || q ? null : folder);
  const usage = useDriveUsage();
  const s3 = useS3Status();
  const createFolder = useCreateFolder();
  const update = useUpdateItem();
  const move = useMoveItems();
  const trashItems = useTrashItems();
  const restore = useRestoreItems();
  const destroy = useDeleteForever();
  const batchLive = useBatchLive();
  const batchMove = useBatchMove();
  const copy = useCopyItems();
  const archive = useArchive();
  const extract = useExtract();
  const addUploads = useUploads((s) => s.add);

  const [layout, setLayoutState] = useState<Layout>(readLayout);
  const [sort, setSort] = useState<Sort>({ key: "name", desc: false });
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [dialog, setDialog] = useState<DialogState>(null);
  const [input, setInput] = useState(q);
  const [dragging, setDragging] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const setLayout = (next: Layout) => {
    setLayoutState(next);
    try {
      localStorage.setItem(LAYOUT_KEY, next);
    } catch {
      /* 记不住就算了 */
    }
  };

  const go = (change: {
    folder?: number | null;
    q?: string | null;
    view?: string | null;
    remote?: string | null;
    ref?: string | null;
  }) =>
    setSearch((prev) => {
      const p = new URLSearchParams(prev);
      const set = (k: string, v: string | null | undefined) => {
        if (v == null || v === "") p.delete(k);
        else p.set(k, v);
      };
      if ("folder" in change)
        set("folder", change.folder ? String(change.folder) : null);
      if ("q" in change) set("q", change.q);
      if ("view" in change) set("view", change.view);
      if ("view" in change && change.view !== "remote") {
        p.delete("remote");
        p.delete("ref");
      }
      if ("remote" in change) set("remote", change.remote);
      if ("ref" in change) set("ref", change.ref);
      return p;
    });

  // 换目录、换分类后清空选择。
  const scopeKey = JSON.stringify(scope);
  useEffect(() => setSelected(new Set()), [scopeKey]);
  useEffect(() => {
    setInput((cur) => (cur.trim() === q ? cur : q));
  }, [q]);
  // 命令面板：/drive?focus=search 聚焦搜索框。
  useEffect(() => {
    if (search.get("focus") !== "search") return;
    searchRef.current?.focus();
    setSearch(
      (prev) => {
        const p = new URLSearchParams(prev);
        p.delete("focus");
        return p;
      },
      { replace: true },
    );
  }, [search]);
  useEffect(() => {
    if (input.trim() === q) return;
    const timer = setTimeout(() => go({ q: input.trim() }), 250);
    return () => clearTimeout(timer);
  }, [input]);

  const list = useMemo(
    () => sortItems(items.data ?? [], sort),
    [items.data, sort],
  );
  const selectedItems = list.filter((i) => selected.has(i.id));
  // 查看器里左右切换的范围：当前列表里的文件。
  const files = useMemo(() => list.filter((i) => !i.isDir), [list]);
  const single =
    selectedItems.length === 1 && !selectedItems[0].isDir
      ? selectedItems[0]
      : null;

  const upload = (files: File[]) => {
    if (files.length === 0 || trash) return;
    addUploads(files, { parent: q ? null : folder, hidden }, () => {
      items.refetch();
      usage.refetch();
    });
  };

  // 拖文件到页面任何位置就上传。
  useEffect(() => {
    if (trash || remoteDrive) return;
    let depth = 0;
    const hasFiles = (e: DragEvent) =>
      Array.from(e.dataTransfer?.types ?? []).includes("Files");
    const enter = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth++;
      setDragging(true);
    };
    const leave = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) setDragging(false);
    };
    const over = (e: DragEvent) => {
      if (hasFiles(e)) e.preventDefault();
    };
    const drop = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      depth = 0;
      setDragging(false);
      upload(Array.from(e.dataTransfer?.files ?? []));
    };
    window.addEventListener("dragenter", enter);
    window.addEventListener("dragleave", leave);
    window.addEventListener("dragover", over);
    window.addEventListener("drop", drop);
    return () => {
      window.removeEventListener("dragenter", enter);
      window.removeEventListener("dragleave", leave);
      window.removeEventListener("dragover", over);
      window.removeEventListener("drop", drop);
    };
  }, [trash, folder, hidden, q, remoteDrive]);

  const open = (item: DriveItem) => {
    if (trash) return;
    if (item.isDir) go({ folder: item.id, q: null });
    else setDialog({ kind: "view", id: item.id, edit: false });
  };

  const download = (targets: DriveItem[]) => {
    // 服务端能打包（B31）时，多个文件或有文件夹就打成一个 zip。
    if (batchLive && (targets.length > 1 || targets.some((i) => i.isDir))) {
      const a = document.createElement("a");
      a.href = zipUrl(targets.map((i) => i.id));
      document.body.appendChild(a);
      a.click();
      a.remove();
      return;
    }
    // 浏览器会拦截太快的连续下载，间隔一下。
    targets
      .filter((i) => !i.isDir)
      .forEach((item, n) =>
        setTimeout(() => {
          const a = document.createElement("a");
          a.href = contentUrl(item.id);
          a.download = item.name;
          document.body.appendChild(a);
          a.click();
          a.remove();
        }, n * 400),
      );
  };

  const doneBatch = (label: string, total: number) => (failed: number) => {
    setSelected(new Set());
    if (failed < total) toast(label);
  };

  const onAction = async (action: ItemAction, item: DriveItem) => {
    switch (action) {
      case "preview":
      case "edit":
        return setDialog({
          kind: "view",
          id: item.id,
          edit: action === "edit",
        });
      case "download":
        return download([item]);
      case "rename":
        return setDialog({ kind: "rename", item });
      case "move":
      case "copy":
      case "compress":
        return setDialog({ kind: action, ids: [item.id], names: [item.name] });
      case "extract":
      case "share":
      case "versions":
        return setDialog({ kind: action, item });
      case "copy-link":
        return navigator.clipboard
          .writeText(new URL(contentUrl(item.id), location.href).href)
          .then(() => toast(t("Link copied")))
          .catch(() => toast({ message: t("Could not copy"), tone: "error" }));
      case "hide":
      case "unhide":
        return update.mutate(
          { id: item.id, body: { hidden: action === "hide" } },
          {
            onSuccess: () =>
              toast(
                action === "hide"
                  ? t("Moved to hidden")
                  : item.restoreTo
                    ? `${t("Restored to")} ${item.restoreTo}`
                    : t("No longer hidden"),
              ),
          },
        );
      case "trash":
        if (
          !(await confirmAction({
            title: `${t("Delete")}“${item.name}”？`,
            description: t("It goes to the trash. You can restore it there."),
          }))
        )
          return;
        return trashItems.mutate([item.id], {
          onSuccess: doneBatch(t("Moved to trash"), 1),
        });
      case "restore":
        return restore.mutate([item.id], {
          onSuccess: doneBatch(t("Restored"), 1),
        });
      case "delete-forever":
        if (
          !(await confirmAction({
            title: `彻底删除“${item.name}”？`,
            description: "删除后不能恢复。",
          }))
        )
          return;
        return destroy.mutate([item.id], {
          onSuccess: doneBatch(t("Deleted"), 1),
        });
    }
  };

  const pick = (kind: "move" | "copy" | "compress") =>
    setDialog({
      kind,
      ids: selectedItems.map((i) => i.id),
      names: selectedItems.map((i) => i.name),
    });

  const transfer = (
    kind: "move" | "copy",
    ids: number[],
    targetId: number,
    conflict: "skip" | "overwrite" | "rename",
  ) => {
    const after = {
      onSuccess: () => (setDialog(null), setSelected(new Set())),
    };
    if (kind === "copy") return copy.mutate({ ids, targetId, conflict }, after);
    // 服务端还没有批量移动时，退回到一个一个改 parentId。
    if (!batchLive)
      return move.mutate(
        { ids, parentId: targetId },
        {
          onSuccess: (failed) => {
            after.onSuccess();
            if (failed < ids.length) toast(t("Moved"));
          },
        },
      );
    batchMove.mutate({ ids, targetId, conflict }, after);
  };

  const toggle = (id: number) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const allSelected = list.length > 0 && selected.size === list.length;

  const path = current.data?.path ?? [];
  const where = trash
    ? t("Trash")
    : q
      ? `${t("Search")}: ${q}`
      : folder
        ? (current.data?.name ?? "")
        : hidden
          ? t("Hidden items")
          : t("Drive");

  return (
    <div className="xc-page drive-page">
      <PageHeading
        title={t("Drive")}
        subtitle={
          <DriveSummary
            usage={usage.data}
            s3={s3.data}
            s3Missing={s3.isError}
            language={language}
          />
        }
        aside={
          <>
            {!trash && !special && (
              <>
                <button
                  className="xc-btn"
                  title={t("New folder")}
                  onClick={() => setDialog({ kind: "new-folder" })}
                  disabled={!!q}
                >
                  <FolderPlus size={15} />
                  <span className="drive-btn-text">{t("New folder")}</span>
                </button>
                <button
                  className="xc-btn primary"
                  title={t("Upload")}
                  onClick={() => fileRef.current?.click()}
                >
                  <Upload size={15} />
                  <span className="drive-btn-text">{t("Upload")}</span>
                </button>
                <input
                  ref={fileRef}
                  type="file"
                  multiple
                  hidden
                  data-testid="drive-file-input"
                  onChange={(e) => {
                    upload(Array.from(e.target.files ?? []));
                    e.target.value = "";
                  }}
                />
              </>
            )}
            {/* 用户 2026-10-10 要求：搜索和视图切换跟按钮放在顶栏，省掉页面里的一行 */}
            {!special && (
              <>
                {!trash && (
                  <SearchBox
                    ref={searchRef}
                    className={`drive-top-search${input ? " has-value" : ""}`}
                    value={input}
                    onChange={setInput}
                    placeholder={t("Search files")}
                    clearLabel={t("Clear")}
                  />
                )}
                <Segmented
                  label={t("View")}
                  value={layout}
                  onChange={setLayout}
                  options={[
                    {
                      value: "list",
                      label: t("List view"),
                      icon: List,
                      iconOnly: true,
                    },
                    {
                      value: "grid",
                      label: t("Grid view"),
                      icon: LayoutGrid,
                      iconOnly: true,
                    },
                  ]}
                />
              </>
            )}
          </>
        }
      />

      {!trash && !q && !special && (
        <nav className="drive-crumbs" aria-label={t("Path")}>
          <button onClick={() => go({ folder: null })}>
            {hidden ? <EyeOff size={13} /> : <HardDrive size={13} />}
            {hidden ? t("Hidden items") : t("Drive")}
          </button>
          {path.map((p) => (
            <span key={p.id}>
              <ChevronRight size={12} />
              <button onClick={() => go({ folder: p.id })}>{p.name}</button>
            </span>
          ))}
          {folder && current.data && (
            <span>
              <ChevronRight size={12} />
              <strong>{current.data.name}</strong>
            </span>
          )}
        </nav>
      )}
      {trash && (
        <p className="drive-muted drive-hint">
          回收站里的文件 30 天后自动清掉。
        </p>
      )}

      {selected.size > 0 && !special && (
        <div
          className="drive-selection"
          role="toolbar"
          aria-label={t("Selected")}
        >
          <strong>
            {t("Selected")} {selected.size}
          </strong>
          <span className="xc-spacer" />
          {trash ? (
            <>
              <button
                className="xc-btn small"
                onClick={() =>
                  restore.mutate([...selected], {
                    onSuccess: doneBatch(t("Restored"), selected.size),
                  })
                }
              >
                <RotateCcw size={14} /> {t("Restore")}
              </button>
              <button
                className="xc-btn small danger"
                onClick={async () => {
                  if (
                    !(await confirmAction({
                      title: `彻底删除选中的 ${selected.size} 项？`,
                      description: "删除后不能恢复。",
                    }))
                  )
                    return;
                  destroy.mutate([...selected], {
                    onSuccess: doneBatch(t("Deleted"), selected.size),
                  });
                }}
              >
                <Trash2 size={14} /> {t("Delete forever")}
              </button>
            </>
          ) : (
            <>
              {single && (
                <button
                  className="xc-btn small"
                  onClick={() =>
                    setDialog({ kind: "view", id: single.id, edit: false })
                  }
                >
                  <FileText size={14} /> {t("Preview")}
                </button>
              )}
              {single && canEdit(single) && (
                <button
                  className="xc-btn small"
                  onClick={() =>
                    setDialog({ kind: "view", id: single.id, edit: true })
                  }
                >
                  <SquarePen size={14} /> {t("Edit")}
                </button>
              )}
              {(batchLive || selectedItems.some((i) => !i.isDir)) && (
                <button
                  className="xc-btn small"
                  onClick={() => download(selectedItems)}
                >
                  <Download size={14} /> {t("Download")}
                </button>
              )}
              <button className="xc-btn small" onClick={() => pick("move")}>
                <FolderInput size={14} /> {t("Move")}
              </button>
              <button className="xc-btn small" onClick={() => pick("copy")}>
                <Copy size={14} /> {t("Copy to")}
              </button>
              <button className="xc-btn small" onClick={() => pick("compress")}>
                <FileArchive size={14} /> {t("Compress")}
              </button>
              <button
                className="xc-btn small danger"
                onClick={async () =>
                  (await confirmAction({
                    title: `${t("Delete")} ${selected.size} ${t("selected items")}？`,
                    description: t(
                      "It goes to the trash. You can restore it there.",
                    ),
                  })) &&
                  trashItems.mutate([...selected], {
                    onSuccess: doneBatch(t("Moved to trash"), selected.size),
                  })
                }
              >
                <Trash2 size={14} /> {t("Delete")}
              </button>
            </>
          )}
          <button
            className="xc-btn ghost small"
            aria-label={t("Clear selection")}
            onClick={() => setSelected(new Set())}
          >
            <X size={14} />
          </button>
        </div>
      )}

      {remoteDrive ? (
        <RemoteView
          drive={remoteDrive}
          folder={remoteRef}
          onOpen={(ref) => go({ ref: ref || null })}
        />
      ) : (
        <section className="drive-body" aria-label={where}>
          {sharesTab ? (
            <SharesView />
          ) : items.isPending ? (
            <Loading />
          ) : items.isError ? (
            <ErrorState error={items.error} onRetry={() => items.refetch()} />
          ) : list.length === 0 ? (
            <EmptyState
              title={
                q
                  ? t("No matching files")
                  : trash
                    ? t("Trash is empty")
                    : t("Nothing here yet")
              }
              icon={trash ? <Trash2 size={26} /> : <HardDrive size={26} />}
            >
              {!q && !trash && (
                <span className="drive-muted">
                  把文件拖到这里，或者点“上传”。
                </span>
              )}
            </EmptyState>
          ) : layout === "list" ? (
            <div className="drive-list" role="table" aria-label={where}>
              <div className="drive-row drive-head" role="row">
                <span role="columnheader">
                  <input
                    type="checkbox"
                    aria-label={t("Select all")}
                    checked={allSelected}
                    onChange={() =>
                      setSelected(
                        allSelected
                          ? new Set()
                          : new Set(list.map((i) => i.id)),
                      )
                    }
                  />
                </span>
                <SortHeader
                  label={t("Name")}
                  col="name"
                  sort={sort}
                  setSort={setSort}
                />
                <SortHeader
                  label={t("Size")}
                  col="size"
                  sort={sort}
                  setSort={setSort}
                />
                <SortHeader
                  label={trash ? t("Auto delete") : t("Modified")}
                  col="updatedAt"
                  sort={sort}
                  setSort={setSort}
                />
                <span role="columnheader" />
                <span role="columnheader" />
              </div>
              {list.map((item) => (
                <div
                  key={item.id}
                  className={`drive-row${selected.has(item.id) ? " selected" : ""}`}
                  role="row"
                >
                  <span role="cell">
                    <input
                      type="checkbox"
                      aria-label={`${t("Select")} ${item.name}`}
                      checked={selected.has(item.id)}
                      onChange={() => toggle(item.id)}
                    />
                  </span>
                  <span role="cell" className="drive-name">
                    <button onClick={() => open(item)} title={item.name}>
                      <FileIcon item={item} />
                      <span>{item.name}</span>
                      {hidden && item.restoreTo && (
                        <small
                          className="drive-restore"
                          title={t("Restores to this folder")}
                        >
                          {t("Originally in")} {item.restoreTo}
                        </small>
                      )}
                    </button>
                    <small className="drive-sub">
                      {item.isDir ? t("Folder") : formatBytes(item.size)} ·{" "}
                      {trash && item.trashedAt
                        ? `${daysLeftInTrash(item.trashedAt)} 天后清掉`
                        : relativeTime(item.updatedAt, language)}
                    </small>
                  </span>
                  <span role="cell" className="drive-col">
                    {item.isDir ? "—" : formatBytes(item.size)}
                  </span>
                  <span role="cell" className="drive-col">
                    {trash && item.trashedAt
                      ? `${daysLeftInTrash(item.trashedAt)} 天后清掉`
                      : relativeTime(item.updatedAt, language)}
                  </span>
                  <span role="cell" className="drive-col">
                    <SyncIcon item={item} />
                  </span>
                  <span role="cell">
                    <ItemMenu
                      item={item}
                      actions={itemActions(item, {
                        trash,
                        vaultUnlocked,
                        hiddenView: hidden,
                        batchLive,
                      })}
                      onAction={onAction}
                    />
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="drive-grid" role="list" aria-label={where}>
              {list.map((item) => (
                <div
                  key={item.id}
                  role="listitem"
                  className={`drive-tile${selected.has(item.id) ? " selected" : ""}`}
                >
                  <input
                    type="checkbox"
                    className="drive-tile-check"
                    aria-label={`${t("Select")} ${item.name}`}
                    checked={selected.has(item.id)}
                    onChange={() => toggle(item.id)}
                  />
                  <button
                    className="drive-tile-main"
                    onClick={() => open(item)}
                    title={item.name}
                  >
                    <span className="drive-thumb">
                      {fileKind(item) === "image" && !trash ? (
                        <Thumb item={item} />
                      ) : (
                        <FileIcon item={item} size={34} />
                      )}
                    </span>
                    <span className="drive-tile-name">{item.name}</span>
                    <small className="drive-muted">
                      {item.isDir ? t("Folder") : formatBytes(item.size)}
                    </small>
                  </button>
                  <div className="drive-tile-menu">
                    <ItemMenu
                      item={item}
                      actions={itemActions(item, {
                        trash,
                        vaultUnlocked,
                        hiddenView: hidden,
                        batchLive,
                      })}
                      onAction={onAction}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {dragging && (
        <div className="drive-drop" aria-hidden>
          <Upload size={28} />
          <strong>松开上传到“{where}”</strong>
        </div>
      )}
      <div className="drive-dock">
        <TaskPanel
          onOpenFolder={(id) => go({ folder: id || null, view: null, q: null })}
        />
        <UploadPanel />
      </div>

      {dialog?.kind === "new-folder" && (
        <NameDialog
          title={t("New folder")}
          initial=""
          submitLabel={t("Create")}
          busy={createFolder.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(name) =>
            createFolder.mutate(
              {
                name,
                parentId: folder ?? undefined,
                hidden: hidden || undefined,
              },
              { onSuccess: () => setDialog(null) },
            )
          }
        />
      )}
      {dialog?.kind === "rename" && (
        <NameDialog
          title={t("Rename")}
          initial={dialog.item.name}
          submitLabel={t("Save")}
          busy={update.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(name) =>
            update.mutate(
              { id: dialog.item.id, body: { name } },
              { onSuccess: () => setDialog(null) },
            )
          }
        />
      )}
      {(dialog?.kind === "move" || dialog?.kind === "copy") && (
        <TransferDialog
          mode={dialog.kind}
          names={dialog.names}
          exclude={dialog.ids}
          hidden={hidden}
          busy={move.isPending || batchMove.isPending || copy.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(targetId, conflict) =>
            transfer(
              dialog.kind === "copy" ? "copy" : "move",
              dialog.ids,
              targetId,
              conflict,
            )
          }
        />
      )}
      {dialog?.kind === "compress" && (
        <ArchiveDialog
          count={dialog.ids.length}
          initialName={archiveName(dialog.names)}
          busy={archive.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(name, format) =>
            archive.mutate(
              {
                ids: dialog.ids,
                name,
                format,
                parentId: (q ? undefined : folder) ?? undefined,
              },
              {
                onSuccess: () => (setDialog(null), setSelected(new Set())),
              },
            )
          }
        />
      )}
      {dialog?.kind === "extract" && (
        <ExtractDialog
          name={dialog.item.name}
          busy={extract.isPending}
          onClose={() => setDialog(null)}
          onPick={() => setDialog({ kind: "extract-pick", item: dialog.item })}
          onHere={() =>
            extract.mutate(
              { id: dialog.item.id },
              { onSuccess: () => setDialog(null) },
            )
          }
        />
      )}
      {dialog?.kind === "extract-pick" && (
        <TransferDialog
          mode="extract"
          names={[dialog.item.name]}
          exclude={[]}
          hidden={hidden}
          busy={extract.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(targetId, conflict) =>
            extract.mutate(
              { id: dialog.item.id, targetId, conflict },
              { onSuccess: () => setDialog(null) },
            )
          }
        />
      )}
      {dialog?.kind === "share" && (
        <ShareDialog item={dialog.item} onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === "versions" && (
        <VersionsDialog item={dialog.item} onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === "view" && (
        <FileViewer
          items={files}
          initialId={dialog.id}
          initialEdit={dialog.edit}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  );
}

/** 图片缩略图。加载失败（比如还没生成）时显示类型图标。 */
function Thumb({ item }: { item: DriveItem }) {
  const [failed, setFailed] = useState(false);
  if (failed) return <FileIcon item={item} size={34} />;
  return (
    <img
      src={thumbnailUrl(item)}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}

function SortHeader({
  label,
  col,
  sort,
  setSort,
}: {
  label: string;
  col: SortKey;
  sort: Sort;
  setSort: (s: Sort) => void;
}) {
  const active = sort.key === col;
  return (
    <span
      role="columnheader"
      className={col === "name" ? "" : "drive-col"}
      aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
    >
      <button
        className="drive-sort"
        onClick={() => setSort(nextSort(sort, col))}
      >
        {label}
        {active &&
          (sort.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}
      </button>
    </span>
  );
}

const S3_LABELS: Record<S3Status["state"], string> = {
  off: "Not set up",
  idle: "Synced",
  syncing: "Syncing",
  failed: "Sync failed",
};

/** 标题下面一行：文件数、占用、S3 同步状态。 */
function DriveSummary({
  usage,
  s3,
  s3Missing,
  language,
}: {
  usage?: { files: number; bytes: number; trashBytes: number };
  s3?: S3Status;
  s3Missing: boolean;
  language: "zh" | "en";
}): ReactNode {
  const t = useT();
  const state = s3?.state ?? "off";
  return (
    <span className="drive-summary">
      <span>
        <Files size={13} /> {usage ? usage.files : "-"} {t("files")}
      </span>
      <span>
        <HardDrive size={13} /> {usage ? formatBytes(usage.bytes) : "-"}
        {usage && usage.trashBytes > 0 && (
          <small>
            {" "}
            · {t("Trash")} {formatBytes(usage.trashBytes)}
          </small>
        )}
      </span>
      {!s3Missing && (
        <span className={`drive-summary-s3 is-${state}`}>
          <Cloud size={13} />{" "}
          {state === "off" ? (
            <Link to="/settings/storage">{t("Set up S3 sync")}</Link>
          ) : (
            <>
              {t(S3_LABELS[state])}
              {s3 && s3.pending > 0 && ` · ${s3.pending} ${t("waiting")}`}
              {state === "idle" && s3?.lastRunAt && (
                <small> · {relativeTime(s3.lastRunAt, language)}</small>
              )}
            </>
          )}
        </span>
      )}
    </span>
  );
}
