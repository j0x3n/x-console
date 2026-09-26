import type * as Model from "../../../types/domain";
import React from "react";
import AccountCard from "./AccountCard";
import AccountIcon from "./AccountIcon";
import { roleZh } from "../company-config";

interface CompanyPeopleProps {
  L: Model.Localize;
  people: Model.PersonRecord[];
  navigate: Model.Navigate;
  detail: Model.CompanyDetail | undefined;
}

export default function CompanyPeople({
  L,
  people,
  navigate,
  detail,
}: CompanyPeopleProps) {
  return (
    <AccountCard title={L("People", "联系人")} count={people.length}>
      <div className="ws-account-list">
        {people.map((person) => (
          <button
            className="ws-account-person-row"
            key={person.name}
            onClick={() => navigate(person.name)}
          >
            <AccountIcon name={person.name} />
            <span>
              <b>{person.name}</b>
              <em>{L(person.role, roleZh[person.role])}</em>
              <small>
                {detail?.peopleTitles?.[person.name] || person.role}
              </small>
            </span>
            <span className="ws-account-person-activity">
              <span
                className={`ws-warmth-label ${person.warmth.toLowerCase()}`}
              >
                ▮▮▮{" "}
                {L(
                  person.warmth,
                  { Warm: "活跃", Neutral: "一般", Cold: "冷淡" }[
                    person.warmth
                  ],
                )}
              </span>
              <small>
                <AccountIcon name={person.knownBy} />
                {person.touch}
              </small>
            </span>
          </button>
        ))}
      </div>
    </AccountCard>
  );
}
