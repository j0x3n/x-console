import type * as Model from "../../types/domain";
import React from "react";
import { companyRecords } from "../../data/workspace";
import PersonIcon from "../../components/ui/PersonIcon";
import { rolesZh } from "../../data/catalogs";
import { Plus, ArrowRight } from "lucide-react";
import CompanyIcon from "../../components/ui/CompanyIcon";
import AgentIcon from "../../components/ui/AgentIcon";
import PersonTimeline from "./components/PersonTimeline";

interface PersonPageProps {
  person: Model.PersonRecord;
  L: Model.Localize;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  activity: Model.ActivityRow[];
}

export default function PersonPage({
  person,
  L,
  navigate,
  openDelegate,
  activity,
}: PersonPageProps) {
  const company = companyRecords.find((row) => row.name === person.company);
  if (!company) return null;
  return (
    <div className="workspace-page ws-person-page">
      <div className="ws-company-heading">
        <PersonIcon name={person.name} />
        <div>
          <h1>{person.name}</h1>
          <p>
            {L(person.role, rolesZh[person.role])} · {person.company}
          </p>
        </div>
        <button className="ws-secondary" onClick={openDelegate}>
          <Plus size={14} />
          {L("Delegate", "委派")}
        </button>
      </div>
      <div className="ws-account-band">
        <div>
          <small>{L("COMPANY", "公司")}</small>
          <button
            className="ws-record-link"
            onClick={() => navigate(person.company)}
          >
            <CompanyIcon company={person.company} />
            {person.company}
            <ArrowRight size={13} />
          </button>
        </div>
        <div>
          <small>{L("WARMTH", "联系热度")}</small>
          <strong>
            {L(
              person.warmth,
              { Warm: "活跃", Neutral: "一般", Cold: "冷淡" }[person.warmth],
            )}
          </strong>
        </div>
        <div>
          <small>{L("KNOWN BY", "熟悉该联系人")}</small>
          <span>
            <AgentIcon name={person.knownBy} /> {person.knownBy}
          </span>
        </div>
        <div>
          <small>{L("NEXT STEP", "下一步")}</small>
          <b>{L(company.next, company.nextZh)}</b>
        </div>
      </div>
      <div className="ws-account-grid">
        <PersonTimeline L={L} activity={activity} person={person} />
        <aside className="ws-account-side">
          <section>
            <h2>{L("Company context", "公司背景")}</h2>
            <p>{L(company.signal, company.signalZh)}</p>
            <button
              className="ws-record-link"
              onClick={() => navigate(person.company)}
            >
              {L("Open company record", "打开公司档案")}
              <ArrowRight size={13} />
            </button>
          </section>
        </aside>
      </div>
    </div>
  );
}
