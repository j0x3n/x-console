import { Link } from "react-router";
import { Play } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { useCardSummary } from "./api";
import "./i18n";
import "./cards.css";

/** 今日页的“待复习”卡片（B130）：今天还要复习几张，点开始进复习页。 */
export default function TodayCardsCard() {
  const t = useT();
  const q = useCardSummary();
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const s = q.data;
  return (
    <div className="cards-today">
      <Link to="/cards" className="cards-today-count">
        <strong>{s.due}</strong>
        <span>{t("cards to review today")}</span>
      </Link>
      {s.due > 0 ? (
        <>
          <small className="xc-muted">
            {s.dueReview} {t("to refresh")} · {s.dueNew} {t("new cards")}
          </small>
          <Link to="/cards" className="xc-btn small primary">
            <Play size={13} /> {t("Start review")}
          </Link>
        </>
      ) : (
        <small className="xc-muted">{t("Nothing to review right now")}</small>
      )}
    </div>
  );
}
