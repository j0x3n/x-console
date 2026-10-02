import { Fragment, useState } from "react";
import { Copy, History, Link2, Trash2 } from "lucide-react";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import {
  isNotLive,
  useDeleteShare,
  useDriveShares,
  type DriveShare,
} from "../api";
import FileIcon from "./FileIcon";
import { cancelShare, copyShare } from "./ShareDialog";
import ShareDownloads from "./ShareDownloads";

/**
 * 分享管理（B31）：云盘页的“分享”标签。
 * 列出所有链接、打开次数、下载次数、最近打开时间，可以复制和取消。失效的链接灰着显示，服务端 7 天后清掉。
 * 点“下载记录”展开最近 20 次下载（B75）。
 */
export default function SharesView() {
  const t = useT();
  const language = useLanguage();
  const shares = useDriveShares();
  const remove = useDeleteShare();
  const [open, setOpen] = useState<number | null>(null);

  if (shares.isPending) return <Loading />;
  if (shares.isError)
    return isNotLive(shares.error) ? (
      <NotLive name={t("Share links")} icon={<Link2 size={28} />} />
    ) : (
      <ErrorState error={shares.error} onRetry={() => shares.refetch()} />
    );
  if (shares.data.length === 0)
    return (
      <EmptyState title={t("No share links")} icon={<Link2 size={26} />}>
        <span className="drive-muted">
          在文件的“更多”菜单里点“分享”，就能生成链接。
        </span>
      </EmptyState>
    );

  const expiry = (s: DriveShare) =>
    s.expiresAt ? relativeTime(s.expiresAt, language) : t("Never expires");

  return (
    <div
      className="drive-list drive-shares"
      role="table"
      aria-label={t("Share links")}
    >
      <div className="drive-row drive-head" role="row">
        <span role="columnheader">{t("Name")}</span>
        <span role="columnheader" className="drive-col">
          {t("Expires")}
        </span>
        <span role="columnheader" className="drive-col">
          {t("Opened times")}
        </span>
        <span role="columnheader" className="drive-col">
          {t("Downloads")}
        </span>
        <span role="columnheader" />
      </div>
      {shares.data.map((s) => (
        <Fragment key={s.id}>
          <div
            className={`drive-row${s.active ? "" : " is-inactive"}`}
            role="row"
          >
            <span role="cell" className="drive-name">
              <span className="drive-share-name">
                <FileIcon item={{ isDir: s.isDir, name: s.itemName }} />
                <span>{s.itemName}</span>
                {!s.active && <span className="xc-badge">{t("Inactive")}</span>}
              </span>
              <small className="drive-sub drive-share-url" title={s.url}>
                {s.url}
                {s.code ? ` · ${t("Password")} ${s.code}` : ""}
              </small>
              <small className="drive-sub">
                {expiry(s)} · {t("Opened times")} {s.visits} · {t("Downloads")}{" "}
                {s.downloads}
                {s.maxDownloads ? ` / ${s.maxDownloads}` : ""}
              </small>
              {s.lastAccessAt && (
                <small className="drive-share-last">
                  {t("Last opened")} {relativeTime(s.lastAccessAt, language)}
                </small>
              )}
            </span>
            <span role="cell" className="drive-col">
              {expiry(s)}
            </span>
            <span role="cell" className="drive-col">
              {s.visits}
            </span>
            <span role="cell" className="drive-col">
              {s.downloads}
              {s.maxDownloads ? ` / ${s.maxDownloads}` : ""}
            </span>
            <span role="cell" className="drive-share-actions">
              <button
                type="button"
                className="xc-btn ghost small"
                title={t("Download history")}
                aria-label={`${t("Download history")} ${s.itemName}`}
                aria-expanded={open === s.id}
                onClick={() => setOpen(open === s.id ? null : s.id)}
              >
                <History size={14} />
              </button>
              {s.active && (
                <button
                  type="button"
                  className="xc-btn ghost small"
                  title={t("Copy link")}
                  aria-label={`${t("Copy link")} ${s.itemName}`}
                  onClick={() => void copyShare(s, t)}
                >
                  <Copy size={14} />
                </button>
              )}
              <button
                type="button"
                className="xc-btn ghost small danger"
                title={t("Cancel share")}
                aria-label={`${t("Cancel share")} ${s.itemName}`}
                onClick={() => void cancelShare(s, remove.mutate, t)}
              >
                <Trash2 size={14} />
              </button>
            </span>
          </div>
          {open === s.id && (
            <div className="drive-share-dl-row" role="row">
              <div role="cell">
                <ShareDownloads shareId={s.id} isDir={s.isDir} />
              </div>
            </div>
          )}
        </Fragment>
      ))}
    </div>
  );
}
