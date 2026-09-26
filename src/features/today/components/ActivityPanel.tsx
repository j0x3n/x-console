import type * as Model from "../../../types/domain";
import React from "react";
import { useT } from "../../../contexts/LanguageContext";
import { ArrowRight } from "lucide-react";
import AgentBadge from "../../../components/ui/AgentBadge";
import CompanyBadge from "../../../components/ui/CompanyBadge";

interface ActivityPanelProps {
  navigate: Model.Navigate;
  activity: Model.ActivityRow[];
}

export default function ActivityPanel({
  navigate,
  activity,
}: ActivityPanelProps) {
  const t = useT();
  return (
    <section className="activity-panel">
      <div className="section-heading">
        <h2>
          {t("Crew activity")} <span className="live-dot" />{" "}
          <span className="live-text">{t("Live")}</span>
        </h2>
        <button className="heading-link" onClick={() => navigate("Crew")}>
          {t("142 runs today")} <ArrowRight size={13} />
        </button>
      </div>
      <div className="activity-list">
        {activity.map((row, index) => (
          <button
            key={`${row[0]}-${index}`}
            className="activity-row"
            onClick={() => navigate(row[2])}
          >
            <AgentBadge name={row[0]} size="small" />
            <span className="activity-content">
              <span>
                <b>{row[0]}</b> {t(row[1])}
              </span>
              <small>
                <CompanyBadge
                  initials={row[2]
                    .split(" ")
                    .map((w) => w[0])
                    .join("")
                    .slice(0, 2)}
                  color="terra"
                />{" "}
                {row[2]} · {t(row[3])}
              </small>
            </span>
            <time>{t(row[4])}</time>
          </button>
        ))}
      </div>
    </section>
  );
}
