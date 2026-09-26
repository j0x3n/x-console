import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  Download,
  File as FileIcon,
  Folder,
  FolderPlus,
  Link2,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage, unwrap } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes, formatDate, formatTime } from "../../../lib/time";
import {
  downloadUrl,
  hostsApi,
  hostsKeys,
  uploadFile,
  useFiles,
  type FileEntry,
  type HostDetail,
} from "../api";
import { breadcrumbs, joinPath } from "../lib";

const MAX_UPLOAD = 1 << 30;

export default function FilesTab({ host }: { host: HostDetail }) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const [path, setPath] = useState("");
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const files = useFiles(host.id, path);
  const list = files.data;
  const sep = list?.sep ?? "/";
  const refresh = () => qc.invalidateQueries({ queryKey: hostsKeys.files(host.id) });
  // 路径框跟着实际打开的目录走（空路径会被代理换成主目录）。
  useEffect(() => {
    if (list?.path) setTyped(list.path);
  }, [list?.path]);

  const go = (p: string) => {
    setPath(p);
    setTyped(p);
  };
  const guard = async (label: string, fn: () => Promise<unknown>, done: string) => {
    setBusy(label);
    try {
      await withElevation(fn);
      toast(done);
      refresh();
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    } finally {
      setBusy("");
    }
  };
  const onUpload = async (fileList: FileList | null) => {
    if (!fileList || !list) return;
    for (const f of Array.from(fileList)) {
      if (f.size > MAX_UPLOAD) {
        toast({ message: `${f.name}: ${t("Files larger than 1 GB are not supported")}`, tone: "error" });
        continue;
      }
      await guard(f.name, () => uploadFile(host.id, joinPath(list.path, f.name, sep), f), `${t("Uploaded")} ${f.name}`);
    }
    if (input.current) input.current.value = "";
  };
  const onDelete = (e: FileEntry) => {
    const dir = e.type === "dir";
    if (!confirm(dir ? `删除文件夹 ${e.name} 和里面的所有内容？` : `删除 ${e.name}？`)) return;
    void guard(
      e.name,
      () =>
        unwrap(
          hostsApi.DELETE("/hosts/{hostId}/files", {
            params: { path: { hostId: host.id }, query: { path: e.path, recursive: dir } },
          }),
        ),
      t("Deleted"),
    );
  };
  const onRename = (e: FileEntry) => {
    const name = prompt(t("New name"), e.name);
    if (!name || name === e.name || !list) return;
    void guard(
      e.name,
      () =>
        unwrap(
          hostsApi.POST("/hosts/{hostId}/files/rename", {
            params: { path: { hostId: host.id } },
            body: { from: e.path, to: joinPath(list.path, name, sep) },
          }),
        ),
      t("Renamed"),
    );
  };
  const onMkdir = () => {
    const name = prompt(t("Folder name"));
    if (!name || !list) return;
    void guard(
      name,
      () =>
        unwrap(
          hostsApi.POST("/hosts/{hostId}/files/mkdir", {
            params: { path: { hostId: host.id } },
            body: { path: joinPath(list.path, name, sep) },
          }),
        ),
      t("Folder created"),
    );
  };

  const crumbs = list ? breadcrumbs(list.path, sep) : [];
  return (
    <div className="xc-card">
      <div className="xc-card-head servers-wrap">
        <div className="xc-row servers-crumbs">
          <button className="xc-btn small ghost" disabled={!list?.parent} onClick={() => list && go(list.parent)} aria-label={t("Up")}>
            <ArrowUp size={14} />
          </button>
          {host.os === "windows" && (
            <button className="xc-btn small ghost" onClick={() => go("/")}>
              {t("Drives")}
            </button>
          )}
          {crumbs.map((c, i) => (
            <span key={c.path} className="xc-row">
              {i > 0 && sep === "/" && crumbs[i - 1].label !== "/" && <span className="xc-muted">/</span>}
              {i > 0 && sep !== "/" && <span className="xc-muted">\</span>}
              <button className="servers-crumb" onClick={() => go(c.path)}>
                {c.label}
              </button>
            </span>
          ))}
        </div>
        <div className="xc-row">
          <button className="xc-btn small" onClick={onMkdir} disabled={!list || (list.path === "/" && host.os === "windows")}>
            <FolderPlus size={14} /> {t("New folder")}
          </button>
          <button className="xc-btn small primary" onClick={() => input.current?.click()} disabled={!list || !!busy}>
            <Upload size={14} /> {busy ? t("Working") : t("Upload")}
          </button>
          <input ref={input} type="file" multiple hidden onChange={(e) => onUpload(e.target.files)} />
          <button className="xc-btn small" onClick={refresh} aria-label={t("Refresh")}>
            <RefreshCw size={14} className={files.isFetching ? "servers-spin" : ""} />
          </button>
        </div>
      </div>
      <form
        className="servers-path"
        onSubmit={(e) => {
          e.preventDefault();
          go(typed.trim());
        }}
      >
        <input className="xc-input xc-mono" value={typed} onChange={(e) => setTyped(e.target.value)} placeholder={t("Path")} aria-label={t("Path")} />
      </form>
      {files.isPending ? (
        <Loading />
      ) : files.isError ? (
        <ErrorState error={files.error} onRetry={() => (list ? go(list.path) : go(""))} />
      ) : list!.entries.length === 0 ? (
        <EmptyState title={t("This folder is empty")} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table servers-table">
            <thead>
              <tr>
                <th>{t("Name")}</th>
                <th>{t("Size")}</th>
                <th>{t("Modified")}</th>
                <th className="servers-hide-sm">{t("Mode")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {list!.entries.map((e) => {
                const isDir = e.type === "dir" || e.type === "symlink_dir";
                const Icon = isDir ? Folder : e.type === "symlink" ? Link2 : FileIcon;
                return (
                  <tr key={e.path}>
                    <td>
                      {isDir ? (
                        <button className="servers-file" onClick={() => go(e.path)}>
                          <Icon size={15} /> {e.name}
                        </button>
                      ) : (
                        <span className="servers-file">
                          <Icon size={15} /> {e.name}
                        </span>
                      )}
                    </td>
                    <td className="servers-num">{isDir ? "—" : formatBytes(e.size)}</td>
                    <td className="xc-muted">
                      {e.modTime.startsWith("0001") ? "—" : `${formatDate(e.modTime, language)} ${formatTime(e.modTime, language)}`}
                    </td>
                    <td className="xc-mono xc-muted servers-hide-sm">{e.mode}</td>
                    <td className="servers-actions">
                      {e.type === "file" && (
                        <a className="xc-btn small ghost" href={downloadUrl(host.id, e.path)} download={e.name} aria-label={t("Download")}>
                          <Download size={13} />
                        </a>
                      )}
                      <button className="xc-btn small ghost" onClick={() => onRename(e)} disabled={!!busy} aria-label={t("Rename")}>
                        <Pencil size={13} />
                      </button>
                      <button className="xc-btn small ghost" onClick={() => onDelete(e)} disabled={!!busy} aria-label={t("Delete")}>
                        <Trash2 size={13} />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
