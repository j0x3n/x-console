import type * as Model from "../../../types/domain";
import React from "react";
import {
  shortDate,
  timelineTitlesZh,
  timelineDetailsZh,
} from "../company-config";
import AccountIcon from "./AccountIcon";
import { FileText } from "lucide-react";

interface CompanyTimelineProps {
  L: Model.Localize;
  counts: Record<string, number>;
  timelineFilter: string;
  setTimelineFilter: Model.Setter<string>;
  groups: string[];
  shownEntries: Model.AccountTimelineEntry[];
}

export default function CompanyTimeline({
  L,
  counts,
  timelineFilter,
  setTimelineFilter,
  groups,
  shownEntries,
}: CompanyTimelineProps) {
  return (
    <section
      className="ws-account-timeline"
      aria-labelledby="ws-timeline-heading"
    >
      <div className="ws-account-timeline-head">
        <h2 id="ws-timeline-heading">
          {L("Timeline", "时间线")} <span>{counts.All}</span>
        </h2>
        <div
          className="ws-account-timeline-filter"
          role="radiogroup"
          aria-label={L("Show", "显示")}
        >
          {["All", "Crew", "People"].map((filter) => (
            <button
              key={filter}
              role="radio"
              aria-checked={timelineFilter === filter}
              className={timelineFilter === filter ? "active" : ""}
              onClick={() => setTimelineFilter(filter)}
            >
              {L(
                filter,
                { All: "全部", Crew: "智能团队", People: "人员" }[filter],
              )}{" "}
              <span>{counts[filter]}</span>
            </button>
          ))}
        </div>
      </div>
      {groups.map((group) => (
        <div className="ws-account-day" key={group}>
          <h3>
            {L(
              group,
              group === "Today"
                ? "今天"
                : group === "Yesterday"
                  ? "昨天"
                  : shortDate(group.replace(/^[A-Za-z]+, /, ""), L),
            )}
          </h3>
          <div className="ws-account-day-entries">
            {shownEntries
              .filter((item) => item[0] === group)
              .map((item, index) => (
                <article key={`${group}-${index}`}>
                  <AccountIcon name={item[1]} />
                  <div className="ws-account-event">
                    <div className="ws-account-event-line">
                      <b>{L(item[2], timelineTitlesZh[item[2]])}</b>
                      <time>{item[4]}</time>
                    </div>
                    {item[3] && <p>{L(item[3], timelineDetailsZh[item[3]])}</p>}
                    {item[5] && (
                      <small>
                        <FileText size={12} />
                        {item[5]}
                      </small>
                    )}
                  </div>
                </article>
              ))}
          </div>
        </div>
      ))}
      {!shownEntries.length && (
        <p className="ws-empty-table">
          {L("No entries in this view", "此视图暂无动态")}
        </p>
      )}
    </section>
  );
}
