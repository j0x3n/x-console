import { ArrowDown, ArrowUp, Pencil, RefreshCw, Trash2 } from "lucide-react";
import MoreMenu from "../../components/ui/MoreMenu";
import { Spinner } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import type { QuotaAccount } from "./api";
import {
  accountOrigin,
  isStale,
  resetAtText,
  resetText,
  viewWindow,
} from "./format";

const ERROR_LABELS: Record<string, string> = {
  offline: "Machine offline",
  signed_out: "Sign-in expired",
  unavailable: "Read failed",
  unsupported: "Agent too old",
  host_missing: "Machine removed",
};

/** 一个账号一张卡片：各个窗口的剩余和重置倒计时，或者余额。 */
export default function AccountCard({
  account,
  now,
  refreshing,
  onRefresh,
  onEdit,
  onDelete,
  onMove,
}: {
  account: QuotaAccount;
  now: Date;
  refreshing: boolean;
  onRefresh: () => void;
  onEdit: () => void;
  onDelete: () => void;
  /** 在同一种服务里上移或下移，到头了就不给 */
  onMove: { up?: () => void; down?: () => void };
}) {
  const t = useT();
  const language = useLanguage();
  const stale = isStale(account);
  const failed = account.status === "error";
  const items = [
    ...(onMove.up
      ? [
          {
            key: "up",
            label: t("Move up"),
            icon: <ArrowUp size={14} />,
            onSelect: onMove.up,
          },
        ]
      : []),
    ...(onMove.down
      ? [
          {
            key: "down",
            label: t("Move down"),
            icon: <ArrowDown size={14} />,
            onSelect: onMove.down,
          },
        ]
      : []),
    {
      key: "edit",
      label: t("Edit"),
      icon: <Pencil size={14} />,
      onSelect: onEdit,
    },
    {
      key: "delete",
      label: t("Delete"),
      icon: <Trash2 size={14} />,
      danger: true,
      onSelect: onDelete,
    },
  ];

  return (
    <article
      className={`xc-card quota-card${stale ? " is-stale" : ""}${failed && !stale ? " is-failed" : ""}`}
      aria-label={account.name}
    >
      <header className="quota-card-head">
        <div className="quota-card-title">
          <strong title={account.name}>{account.name}</strong>
          <small title={account.user ?? undefined}>
            {[accountOrigin(account), account.user].filter(Boolean).join(" · ")}
          </small>
        </div>
        {account.plan && (
          <span className="xc-badge accent quota-plan">{account.plan}</span>
        )}
        <button
          className="xc-btn small ghost"
          aria-label={t("Refresh")}
          title={t("Refresh")}
          disabled={refreshing}
          onClick={onRefresh}
        >
          {refreshing ? <Spinner /> : <RefreshCw size={14} />}
        </button>
        <MoreMenu label={t("More")} title={account.name} items={items} />
      </header>

      {failed && (
        <div className="quota-error" role="alert">
          <span className="xc-badge danger">
            {t(ERROR_LABELS[account.errorCode ?? ""] ?? "Read failed")}
          </span>
          {account.errorCode !== "offline" && <span>{account.error}</span>}
        </div>
      )}
      {stale && (
        <small className="quota-stale-note">
          {t("These numbers are from the last successful reading.")}
        </small>
      )}

      {account.windows.length > 0 && (
        <ul className="quota-windows">
          {account.windows.map((w, i) => {
            const v = viewWindow(w, now);
            const reset = resetText(w, now, language);
            return (
              <li key={`${w.name}-${i}`} className={`tone-${v.tone}`}>
                <div className="quota-window-top">
                  <span className="quota-window-name">
                    {w.name}
                    {w.aside && (
                      <small className="quota-aside">{t("Extra")}</small>
                    )}
                  </span>
                  <strong className="quota-left">
                    {t("Left")} {v.remaining}%
                  </strong>
                </div>
                <div
                  className="quota-bar"
                  role="progressbar"
                  aria-label={w.name}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(v.used)}
                >
                  <span style={{ width: `${v.used}%` }} />
                </div>
                {reset && (
                  <small
                    className="quota-reset"
                    title={resetAtText(w, language)}
                  >
                    {reset}
                  </small>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {account.balances.length > 0 && (
        <div className="quota-balances">
          <small>{t("Balance")}</small>
          <div>
            {account.balances.map((b) => (
              <strong key={b.currency} className="quota-balance">
                {b.amount}
                <small>{b.currency}</small>
              </strong>
            ))}
          </div>
        </div>
      )}
      {account.credits && (
        <small className="quota-credits">
          {t("Credits")}: {account.credits}
        </small>
      )}

      {account.status === "pending" && (
        <div className="quota-pending">
          <Spinner /> <small>{t("Reading…")}</small>
        </div>
      )}

      {account.readAt && (
        <footer className="quota-foot">
          {t("Last read")} {relativeTime(account.readAt, language)}
        </footer>
      )}
    </article>
  );
}
