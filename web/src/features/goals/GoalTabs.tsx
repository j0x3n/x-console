import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import "./i18n";

/** 页面上方的切换：目标 / 每周复盘。 */
export default function GoalTabs({ active }: { active: "goals" | "review" }) {
  const t = useT();
  return (
    <div className="xc-tabs" role="tablist">
      <Link
        to="/goals"
        className={active === "goals" ? "active" : ""}
        role="tab"
        aria-selected={active === "goals"}
      >
        {t("Goals")}
      </Link>
      <Link
        to="/goals/review"
        className={active === "review" ? "active" : ""}
        role="tab"
        aria-selected={active === "review"}
      >
        {t("Weekly review")}
      </Link>
    </div>
  );
}
