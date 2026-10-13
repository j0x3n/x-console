import { useEffect, useRef, useState } from "react";
import {
  ChevronRight,
  Cloud,
  Download,
  FolderOpen,
  FolderPlus,
  Upload,
  List,
  LayoutGrid,
  SquarePen,
  Trash2,
} from "lucide-react";
import { apiFetch, unwrap, errorMessage } from "../../../api/client";
import PageActions from "../../../components/layout/PageActions";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import MoreMenu from "../../../components/ui/MoreMenu";
import { SearchBox, Segmented } from "../../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { withElevation } from "../../../auth/elevation";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes, relativeTime } from "../../../lib/time";
import { storageApi } from "../../storage/api";
import {
  remoteDownloadUrl,
  useRemoteItems,
  type RemoteDrive,
  type RemoteDriveEntry,
} from "../remote";
import { sortItems, type Sort } from "../logic";
import type { DriveItem } from "../api";
import FileViewer from "../viewer/FileViewer";
import { canEdit } from "../viewer/kind";
import type { FileSource } from "../viewer/source";
import NameDialog from "./NameDialog";
import FileIcon from "./FileIcon";
import SortHeader from "./SortHeader";

/** 外部网盘使用云盘的列表、网格、文件查看器和名称弹窗。 */
export default function RemoteView({
  drive,
  folder,
  onOpen,
  layout,
  onLayoutChange: setLayout,
}: {
  drive: RemoteDrive;
  folder: string;
  onOpen: (ref: string) => void;
  layout: "list" | "grid";
  onLayoutChange: (layout: "list" | "grid") => void;
}) {
  const t = useT();
  const language = useLanguage();
  const items = useRemoteItems(drive.id, folder);
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<Sort>({ key: "name", desc: false });
  const [dialog, setDialog] = useState<"folder" | RemoteDriveEntry | null>(
    null,
  );
  const [viewer, setViewer] = useState<{ id: number; edit: boolean } | null>(
    null,
  );
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const identities = useRef(new Map<string, number>());
  useEffect(() => {
    setQ("");
    setViewer(null);
    setDialog(null);
  }, [drive.id, folder]);
  const entries = items.data?.items ?? [];
  const makeItem = (entry: RemoteDriveEntry): DriveItem => {
    let id = identities.current.get(entry.ref);
    if (id == null) {
      id = identities.current.size + 1;
      identities.current.set(entry.ref, id);
    }
    return {
      id,
      name: entry.name,
      isDir: entry.isDir,
      size: entry.size ?? 0,
      mime: entry.mime,
      createdAt: entry.modifiedAt ?? "",
      updatedAt: entry.modifiedAt ?? "",
      hidden: false,
      syncState: "off",
    };
  };
  const list = sortItems(
    entries
      .filter((entry) =>
        entry.name.toLowerCase().includes(q.trim().toLowerCase()),
      )
      .map(makeItem),
    sort,
  );
  const byId = (id: number) =>
    entries.find((entry) => identities.current.get(entry.ref) === id)!;
  const url = (id: number, inline = false) =>
    remoteDownloadUrl(drive.id, byId(id).ref) + (inline ? "&inline=true" : "");
  const source: FileSource = {
    url,
    read: async (id, range) => {
      const res = await apiFetch(url(id, true).replace("/api/v1", ""), {
        headers: range ? { Range: range } : undefined,
        cache: "no-store",
      });
      return {
        bytes: new Uint8Array(await res.arrayBuffer()),
        etag: res.headers.get("ETag") ?? "",
      };
    },
    save: async (id, text, etag) => {
      const entry = byId(id);
      const res = await apiFetch(
        `/storage/remotes/${drive.id}/content?ref=${encodeURIComponent(entry.ref)}`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/octet-stream",
            ...(etag ? { "If-Match": etag } : {}),
          },
          body: text,
        },
      );
      const saved: RemoteDriveEntry = await res.json();
      void items.refetch();
      return {
        item: { ...makeItem(saved), id },
        etag: res.headers.get("ETag") ?? "",
      };
    },
  };
  const run = async (action: () => Promise<unknown>) => {
    if (busy) return;
    setBusy(true);
    try {
      await action();
      setDialog(null);
      await items.refetch();
      toast(t("Saved"));
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    } finally {
      setBusy(false);
    }
  };
  const submitName = (name: string) =>
    void run(() =>
      dialog === "folder"
        ? unwrap(
            storageApi.POST("/storage/remotes/{remoteId}/items", {
              params: { path: { remoteId: drive.id } },
              body: { parent: folder, name },
            }),
          )
        : unwrap(
            storageApi.PATCH("/storage/remotes/{remoteId}/items", {
              params: { path: { remoteId: drive.id } },
              body: { ref: (dialog as RemoteDriveEntry).ref, name },
            }),
          ),
    );
  const upload = (files: File[]) =>
    void run(async () => {
      for (const file of files) {
        await apiFetch(
          `/storage/remotes/${drive.id}/content?parent=${encodeURIComponent(folder)}&name=${encodeURIComponent(file.name)}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: file,
          },
        );
      }
    });
  const open = (entry: RemoteDriveEntry, edit = false) =>
    entry.isDir
      ? onOpen(entry.ref)
      : entry.downloadable && setViewer({ id: makeItem(entry).id, edit });
  const remove = async (entry: RemoteDriveEntry) => {
    if (
      !(await confirmAction({
        title: `${t("Delete")} ${entry.name}？`,
        description: "删除会同步到外部网盘。文件夹里的内容也会删除。",
      }))
    )
      return;
    void run(() =>
      withElevation(() =>
        unwrap(
          storageApi.DELETE("/storage/remotes/{remoteId}/items", {
            params: { path: { remoteId: drive.id }, query: { ref: entry.ref } },
          }),
        ),
      ),
    );
  };
  return (
    <>
      <PageActions>
        <SearchBox
          className={`drive-top-search${q ? " has-value" : ""}`}
          value={q}
          onChange={setQ}
          placeholder={t("Search files")}
          clearLabel={t("Clear")}
        />
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
        <button
          className="xc-btn"
          title={t("New folder")}
          disabled={busy}
          onClick={() => setDialog("folder")}
        >
          <FolderPlus size={15} />
          <span className="drive-btn-text">{t("New folder")}</span>
        </button>
        <button
          className="xc-btn primary"
          title={t("Upload")}
          disabled={busy}
          onClick={() => input.current?.click()}
        >
          <Upload size={15} />
          <span className="drive-btn-text">{t("Upload")}</span>
        </button>
        <input
          ref={input}
          type="file"
          hidden
          multiple
          aria-label={t("Upload")}
          onChange={(event) => {
            upload(Array.from(event.target.files ?? []));
            event.target.value = "";
          }}
        />
      </PageActions>
      <nav className="drive-crumbs" aria-label={t("Path")}>
        <button onClick={() => onOpen("")}>
          <Cloud size={13} />
          {drive.name}
        </button>
        {(items.data?.trail ?? []).map((crumb) => (
          <span key={crumb.ref}>
            <ChevronRight size={12} />
            <button onClick={() => onOpen(crumb.ref)}>{crumb.name}</button>
          </span>
        ))}
      </nav>
      {drive.gdrive &&
        (drive.gdrive.limited || drive.gdrive.writable === false) && (
          <p className="drive-muted drive-hint">
            到设置 → 存储重新授权，就能浏览和修改网盘里的全部文件。
          </p>
        )}
      <section
        className="drive-body"
        aria-label={drive.name}
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes("Files"))
            event.preventDefault();
        }}
        onDrop={(event) => {
          if (!event.dataTransfer.files.length) return;
          event.preventDefault();
          upload(Array.from(event.dataTransfer.files));
        }}
      >
        {items.isPending ? (
          <Loading />
        ) : items.isError ? (
          <ErrorState error={items.error} onRetry={() => items.refetch()} />
        ) : list.length === 0 ? (
          <EmptyState
            title={q ? t("No matching files") : t("This folder is empty")}
            icon={<FolderOpen size={28} />}
          />
        ) : (
          <div className={layout === "grid" ? "drive-grid" : "drive-table"}>
            {layout === "list" && (
              <div className="drive-row drive-head">
                <span />
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
                  label={t("Modified")}
                  col="updatedAt"
                  sort={sort}
                  setSort={setSort}
                />
                <span />
                <span />
              </div>
            )}
            {list.map((item) => {
              const entry = byId(item.id);
              const modified = entry.modifiedAt
                ? relativeTime(entry.modifiedAt, language)
                : "";
              return (
                <div
                  key={entry.ref}
                  className={layout === "grid" ? "drive-tile" : "drive-row"}
                >
                  {layout === "list" && <span />}
                  <div className={layout === "grid" ? "" : "drive-name"}>
                    <button
                      className={
                        layout === "grid" ? "drive-tile-main" : undefined
                      }
                      onClick={() => open(entry)}
                      title={item.name}
                    >
                      {layout === "grid" ? (
                        <span className="drive-thumb">
                          <FileIcon item={item} size={48} />
                        </span>
                      ) : (
                        <FileIcon item={item} size={18} />
                      )}
                      <span
                        className={
                          layout === "grid" ? "drive-tile-name" : undefined
                        }
                      >
                        {item.name}
                      </span>
                      {layout === "grid" && (
                        <small className="drive-muted">
                          {item.isDir ? t("Folder") : formatBytes(item.size)}
                        </small>
                      )}
                    </button>
                    {layout === "list" && (
                      <small className="drive-sub">
                        {item.isDir ? t("Folder") : formatBytes(item.size)}
                        {modified && ` · ${modified}`}
                      </small>
                    )}
                  </div>
                  {layout === "list" && (
                    <>
                      <span className="drive-size">
                        {item.isDir ? "–" : formatBytes(item.size)}
                      </span>
                      <span className="drive-modified">{modified}</span>
                    </>
                  )}
                  {layout === "list" && <span />}
                  <div className={layout === "grid" ? "drive-tile-menu" : ""}>
                    <MoreMenu
                      label={`${t("More")}：${item.name}`}
                      items={[
                        ...(!entry.isDir && entry.downloadable
                          ? [
                              {
                                key: "preview",
                                label: t("Preview"),
                                icon: <FolderOpen size={14} />,
                                onSelect: () => open(entry),
                              },
                              ...(canEdit(item)
                                ? [
                                    {
                                      key: "edit",
                                      label: t("Edit"),
                                      icon: <SquarePen size={14} />,
                                      onSelect: () => open(entry, true),
                                    },
                                  ]
                                : []),
                              {
                                key: "download",
                                label: t("Download"),
                                icon: <Download size={14} />,
                                onSelect: () => {
                                  const a = document.createElement("a");
                                  a.href = url(item.id);
                                  a.download = item.name;
                                  a.click();
                                },
                              },
                            ]
                          : []),
                        {
                          key: "rename",
                          label: t("Rename"),
                          icon: <SquarePen size={14} />,
                          onSelect: () => setDialog(entry),
                        },
                        {
                          key: "delete",
                          label: t("Delete"),
                          icon: <Trash2 size={14} />,
                          danger: true,
                          onSelect: () => void remove(entry),
                        },
                      ]}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>
      {dialog && (
        <NameDialog
          key={dialog === "folder" ? "new" : dialog.ref}
          title={t(dialog === "folder" ? "New folder" : "Rename")}
          initial={dialog === "folder" ? "" : dialog.name}
          submitLabel={t(dialog === "folder" ? "Create" : "Save")}
          busy={busy}
          onSubmit={submitName}
          onClose={() => setDialog(null)}
        />
      )}
      {viewer && (
        <FileViewer
          items={list.filter(
            (item) => !item.isDir && byId(item.id).downloadable,
          )}
          initialId={viewer.id}
          initialEdit={viewer.edit}
          source={source}
          onClose={() => setViewer(null)}
        />
      )}
    </>
  );
}
