import { ChevronRight, Cloud, Download, FolderOpen } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, relativeTime } from "../../../lib/time";
import {
  remoteDownloadUrl,
  useRemoteItems,
  type RemoteDrive,
  type RemoteDriveEntry,
} from "../remote";
import FileIcon from "./FileIcon";

/** 云盘页里的一个网盘标签（B68）：路径导航和文件列表，只能浏览和下载。 */
export default function RemoteView({
  drive,
  folder,
  onOpen,
}: {
  drive: RemoteDrive;
  folder: string;
  onOpen: (ref: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const items = useRemoteItems(drive.id, folder);
  const download = (e: RemoteDriveEntry) => {
    window.location.href = remoteDownloadUrl(drive.id, e.ref);
  };
  const list = items.data?.items ?? [];
  return (
    <>
      <nav className="drive-crumbs" aria-label={t("Path")}>
        <button onClick={() => onOpen("")}>
          <Cloud size={13} />
          {drive.name}
        </button>
        {(items.data?.trail ?? []).map((c, i, all) =>
          i === all.length - 1 ? (
            <span key={c.ref}>
              <ChevronRight size={12} />
              <strong>{c.name}</strong>
            </span>
          ) : (
            <span key={c.ref}>
              <ChevronRight size={12} />
              <button onClick={() => onOpen(c.ref)}>{c.name}</button>
            </span>
          ),
        )}
      </nav>
      {drive.gdrive?.limited && (
        <p className="drive-muted drive-hint">
          现在只能看到面板自己建的文件。到 设置 → 存储 里重新授权 Google
          Drive，就能看到网盘里的全部文件。
        </p>
      )}
      <section className="drive-body" aria-label={drive.name}>
        {items.isPending ? (
          <Loading />
        ) : items.isError ? (
          <ErrorState error={items.error} onRetry={() => items.refetch()} />
        ) : list.length === 0 ? (
          <EmptyState
            title={t("Nothing here yet")}
            icon={<FolderOpen size={26} />}
          />
        ) : (
          <div className="drive-list" role="table" aria-label={drive.name}>
            <div className="drive-row drive-head" role="row">
              <span role="columnheader" />
              <span role="columnheader">{t("Name")}</span>
              <span role="columnheader">{t("Size")}</span>
              <span role="columnheader">{t("Modified")}</span>
              <span role="columnheader" />
              <span role="columnheader" />
            </div>
            {list.map((e) => {
              const modified = e.modifiedAt
                ? relativeTime(e.modifiedAt, language)
                : "";
              return (
                <div key={e.ref} className="drive-row" role="row">
                  <span role="cell" />
                  <span role="cell" className="drive-name">
                    <button
                      onClick={() =>
                        e.isDir ? onOpen(e.ref) : e.downloadable && download(e)
                      }
                      title={e.name}
                    >
                      <FileIcon
                        item={{ isDir: e.isDir, name: e.name, mime: "" }}
                      />
                      <span>{e.name}</span>
                    </button>
                    <small className="drive-sub">
                      {e.isDir ? t("Folder") : formatBytes(e.size ?? 0)}
                      {modified && ` · ${modified}`}
                    </small>
                  </span>
                  <span role="cell" className="drive-col">
                    {e.isDir ? "—" : formatBytes(e.size ?? 0)}
                  </span>
                  <span role="cell" className="drive-col">
                    {modified || "—"}
                  </span>
                  <span role="cell" />
                  <span role="cell">
                    {!e.isDir && (
                      <button
                        className="xc-btn ghost small"
                        disabled={!e.downloadable}
                        aria-label={`${t("Download")} ${e.name}`}
                        title={
                          e.downloadable
                            ? t("Download")
                            : "Google 文档这类在线文件不能直接下载"
                        }
                        onClick={() => download(e)}
                      >
                        <Download size={14} />
                      </button>
                    )}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </section>
    </>
  );
}
