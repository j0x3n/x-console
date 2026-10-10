import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { useScreenSummary } from "./api";
import { CATEGORY_COLORS, CATEGORY_LABELS, durationText } from "./format";
import "./i18n";
import "./screentime.css";

const MAX_ROWS = 4;

/** 今日页的“电脑时间”卡片（B116）：今天的总时长和各类别。 */
export default function TodayScreenTimeCard() {
  const t = useT();
  const q = useScreenSummary("day", "", "");
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const s = q.data;
  const max = s.categories[0]?.minutes ?? 0;
  return (
    <Link className="screentime-today" to="/screentime">
      <div className="screentime-today-total">
        <strong>{durationText(t, s.minutes)}</strong>
        <small>{t("Total time")}</small>
      </div>
      {s.categories.slice(0, MAX_ROWS).map((c) => (
        <div key={c.category} className="screentime-today-row">
          <span className="screentime-today-name">
            <i style={{ background: CATEGORY_COLORS[c.category] }} />
            {t(CATEGORY_LABELS[c.category])}
          </span>
          <span className="screentime-track">
            <i
              style={{
                width: `${max > 0 ? (c.minutes / max) * 100 : 0}%`,
                background: CATEGORY_COLORS[c.category],
              }}
            />
          </span>
          <span className="screentime-today-time">
            {durationText(t, c.minutes)}
          </span>
        </div>
      ))}
    </Link>
  );
}
