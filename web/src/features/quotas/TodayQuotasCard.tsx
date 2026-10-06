import { Link } from "react-router";
import { Gauge } from "lucide-react";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { useQuotaAccounts } from "./api";
import { KIND_NAMES, resetText, tightestWindow } from "./format";
import { useNow } from "./useNow";
import "./i18n";
import "./quotas.css";

const MAX_ROWS = 6;

/** 今日页的“AI 额度”卡片（B111）：每个账号一行，显示最紧张的窗口。 */
export default function TodayQuotasCard() {
  const t = useT();
  const language = useLanguage();
  const now = useNow();
  const list = useQuotaAccounts();
  if (list.isPending || list.isError) return <QueryState query={list} />;
  const rows = list.data.slice(0, MAX_ROWS);
  return (
    <div className="xc-list">
      {rows.map((a) => {
        const v = tightestWindow(a, now);
        const bal = a.balances[0];
        const failed = a.status === "error";
        return (
          <Link key={a.id} className="today-row" to="/quotas">
            <span className={`today-row-icon${failed ? " danger" : ""}`}>
              <Gauge size={15} />
            </span>
            <span className="today-row-main">
              <strong>{a.name}</strong>
              <small>
                {failed && !v && !bal
                  ? (a.error ?? t("Read failed"))
                  : v
                    ? [
                        KIND_NAMES[a.kind],
                        v.window.name,
                        resetText(v.window, now, language),
                      ]
                        .filter(Boolean)
                        .join(" · ")
                    : KIND_NAMES[a.kind]}
              </small>
            </span>
            {v ? (
              <span className={`xc-badge ${v.tone}`}>
                {t("Left")} {v.remaining}%
              </span>
            ) : bal ? (
              <strong className="quota-today-value">
                {bal.amount} {bal.currency}
              </strong>
            ) : (
              <span className="xc-badge">—</span>
            )}
          </Link>
        );
      })}
    </div>
  );
}
