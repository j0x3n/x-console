import { Link } from "react-router";
import { ScanEye } from "lucide-react";
import { QueryState } from "../overview/components/shared";
import { useWatchers } from "./api";
import { emptyValue, needsUser, sortWatchers, STATUS } from "./lib";
import "./i18n";
import "./watchers.css";

const MAX_ROWS = 6;

/** 今日页的“网站自动化”卡片（B234）：一行一个任务，等你处理的在前。 */
export default function TodayWatchersCard() {
  const list = useWatchers();
  if (list.isPending || list.isError) return <QueryState query={list} />;
  const all = sortWatchers(list.data);
  const rows = all.slice(0, MAX_ROWS);
  return (
    <div className="xc-list">
      {rows.map((w) => {
        const st = STATUS[w.status];
        return (
          <Link
            key={w.id}
            className="today-row"
            to={`/monitoring/watchers/${w.id}`}
            title={w.currentValue || undefined}
          >
            <span
              className={`today-row-icon${needsUser(w) ? " today-tone-warn" : w.status === "error" ? " danger" : ""}`}
            >
              <ScanEye size={15} />
            </span>
            <span className="today-row-main">
              <strong>{w.name}</strong>
              <small>{w.currentValue || emptyValue(w)}</small>
            </span>
            <span className={`xc-badge ${st.tone}`}>{st.label}</span>
          </Link>
        );
      })}
      {all.length > MAX_ROWS && (
        <Link
          className="today-row watchers-today-more"
          to="/monitoring/watchers"
        >
          还有 {all.length - MAX_ROWS} 个
        </Link>
      )}
    </div>
  );
}
