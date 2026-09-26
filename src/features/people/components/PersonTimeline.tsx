import type * as Model from "../../../types/domain";
import React from "react";
import AgentIcon from "../../../components/ui/AgentIcon";

interface PersonTimelineProps {
  L: Model.Localize;
  activity: Model.ActivityRow[];
  person: Model.PersonRecord;
}

export default function PersonTimeline({
  L,
  activity,
  person,
}: PersonTimelineProps) {
  return (
    <section className="ws-timeline">
      <div className="ws-section-heading">
        <h2>{L("Recent activity", "最近动态")}</h2>
      </div>
      {activity
        .filter((row) => row[2] === person.company)
        .slice(0, 5)
        .map((row, index) => (
          <article className="ws-person-activity" key={index}>
            <AgentIcon name={row[0]} />
            <span>
              {row[0]} {L(row[1], row[1])}
              <small>
                {person.company} · {L(row[3], row[3])}
              </small>
            </span>
          </article>
        ))}
    </section>
  );
}
