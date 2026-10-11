import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { useGoals } from "./api";
import { ProgressBar } from "./GoalCard";
import { STATE_LABELS, currentPeriod, fmtPercent, stateTone } from "./format";
import "./i18n";
import "./goals.css";

const MAX_ROWS = 3;

/** 本季度进行中的目标。今日页用它决定要不要显示“目标”卡片。 */
export function useTodayGoals() {
  return useGoals(currentPeriod(), "active");
}

/** 今日页的“目标”卡片（B128）：进行中的目标、进度，落后的标出来。 */
export default function TodayGoalsCard() {
  const t = useT();
  const q = useTodayGoals();
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const items = q.data.items;
  const more = items.length - MAX_ROWS;
  return (
    <div className="goals-today">
      {items.slice(0, MAX_ROWS).map((g) => (
        <Link key={g.id} to="/goals" className="goals-today-row">
          <span className="goals-today-title">
            <span>{g.title}</span>
            {g.state === "behind" && (
              <span className={`xc-badge ${stateTone(g.state)}`}>
                {t(STATE_LABELS.behind)}
              </span>
            )}
          </span>
          <ProgressBar
            progress={g.progress}
            expected={g.expected}
            label={`${g.title} ${t("Goal progress")}`}
          />
          <small>{fmtPercent(g.progress)}</small>
        </Link>
      ))}
      {more > 0 && (
        <Link to="/goals" className="goals-today-more">
          {`还有 ${more} 个目标`}
        </Link>
      )}
    </div>
  );
}
