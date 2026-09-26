import type * as Model from "../../../types/domain";
import React from "react";
import { useT } from "../../../contexts/LanguageContext";
import AgentBadge from "../../../components/ui/AgentBadge";
import { meetings } from "../../../data/dashboard";
import CompanyBadge from "../../../components/ui/CompanyBadge";

interface MeetingsProps {
  navigate: Model.Navigate;
}

export default function Meetings({ navigate }: MeetingsProps) {
  const t = useT();
  return (
    <section className="meetings-panel">
      <div className="section-heading">
        <h2>
          {t("Meetings")} <span>3</span>
        </h2>
        <span className="section-note">
          <AgentBadge name="Echo" size="tiny" /> {t("Echo preps each one")}
        </span>
      </div>
      <div className="meetings-list">
        {meetings.map((m) => (
          <button
            className="meeting-row"
            key={m.time}
            onClick={() => navigate(m.company)}
          >
            <span className="meeting-time">
              {t(m.time)}
              <small>{t(m.duration)}</small>
            </span>
            <span className="meeting-info">
              <span className="meeting-title">
                <CompanyBadge initials={m.initials} color={m.color} />
                <b>{m.company}</b>
              </span>
              <small>{t(m.note)}</small>
            </span>
            <span className={`meeting-tag ${m.tagClass}`}>{t(m.tag)}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
