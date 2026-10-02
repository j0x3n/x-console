import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatDate, formatTime } from "../../../lib/time";
import { useShareDownloads } from "../api";

/**
 * 一条分享链接最近 20 次下载（B75）：时间、打码的 IP、浏览器、文件名。
 * 展开时才查。
 */
export default function ShareDownloads({
  shareId,
  isDir,
}: {
  shareId: number;
  isDir: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const list = useShareDownloads(shareId, true);
  return (
    <div className="drive-share-dl-wrap">
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 ? (
        <p className="drive-muted">{t("No downloads yet")}</p>
      ) : (
        <ul className="drive-share-dl" aria-label={t("Recent downloads")}>
          {list.data.map((d, i) => (
            <li key={`${d.at}-${i}`}>
              <span>
                {formatDate(d.at, language)} {formatTime(d.at, language)}
              </span>
              <span className="xc-mono">{d.ip}</span>
              <span>{d.userAgent || t("Unknown browser")}</span>
              {isDir && d.itemName && (
                <span className="drive-share-dl-name" title={d.itemName}>
                  {d.itemName}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
