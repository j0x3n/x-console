import { useT } from "../../contexts/LanguageContext";
import { CopyButton } from "../../components/ui/ErrorNotices";
import { formatTime, useErrorStore } from "../../lib/errors";

/** 设置 → 最近的报错（B41）：本次打开页面以来的最近 50 条，可以复制。 */
export default function ErrorsTab() {
  const t = useT();
  const history = useErrorStore((s) => s.history);
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h3>{t("Recent errors")}</h3>
        {history.length > 0 && (
          <CopyButton
            text={history.map((n) => n.detail).join("\n\n----\n\n")}
            label="Copy all"
          />
        )}
      </div>
      {history.length === 0 ? (
        <p className="xc-muted">{t("No errors since this page was opened")}</p>
      ) : (
        <div className="error-history">
          {history.map((n) => (
            <div key={n.id} className="error-history-item">
              <div>
                <strong>
                  {t(n.title)}
                  {n.count > 1 ? ` ×${n.count}` : ""}
                </strong>
                <p title={n.message}>{n.message}</p>
                <time>{formatTime(n.at)}</time>
              </div>
              <CopyButton text={n.detail} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
