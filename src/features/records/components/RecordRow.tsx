import type * as Model from "../../../types/domain";
import React from "react";
import PersonIcon from "../../../components/ui/PersonIcon";
import CompanyIcon from "../../../components/ui/CompanyIcon";
import { rolesZh, stageZh } from "../../../data/catalogs";
import Warmth from "../../../components/ui/Warmth";
import AgentIcon from "../../../components/ui/AgentIcon";
import { relativeTime } from "../../../lib/formatters";
import Health from "../../../components/ui/Health";
import { companyRowMeta, signalTone } from "../record-config";
import { Plus, ArrowUpRight as OpenIcon } from "lucide-react";

interface RecordRowProps {
  row: Model.RecordData;
  navigate: Model.Navigate;
  L: Model.Localize;
  selected: string[];
  setSelected: Model.Setter<string[]>;
  isPeople: boolean;
  company: Model.RecordData | undefined;
  openDelegate: Model.OpenDelegate;
}

export default function RecordRow({
  row,
  navigate,
  L,
  selected,
  setSelected,
  isPeople,
  company,
  openDelegate,
}: RecordRowProps) {
  return (
    <tr
      key={row.name}
      onClick={() => navigate(row.name)}
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter" && event.target === event.currentTarget)
          navigate(row.name);
      }}
    >
      <td>
        <label className="ws-checkbox" onClick={(e) => e.stopPropagation()}>
          <input
            type="checkbox"
            aria-label={L(`Select ${row.name}`, `选择 ${row.name}`)}
            checked={selected.includes(row.name)}
            onChange={() =>
              setSelected((prev) =>
                prev.includes(row.name)
                  ? prev.filter((name) => name !== row.name)
                  : [...prev, row.name],
              )
            }
          />
        </label>
        {isPeople ? (
          <PersonIcon name={row.name} />
        ) : (
          <CompanyIcon company={row.name} />
        )}
        <b>{row.name}</b>
      </td>
      {isPeople ? (
        <>
          <td>
            <CompanyIcon company={row.company ?? ""} />
            {row.company}
          </td>
          <td>
            <span className="ws-stage-pill">
              {L(row.role, rolesZh[row.role ?? ""])}
            </span>
          </td>
          <td>
            <Warmth value={row.warmth ?? "Neutral"} L={L} />
          </td>
          <td>
            <AgentIcon name={row.knownBy ?? ""} />
            {row.knownBy}
          </td>
          <td>{relativeTime(row.touch, L)}</td>
          <td>{L(company?.next || "—", company?.nextZh || "—")}</td>
        </>
      ) : (
        <>
          <td>
            <span className="ws-stage-pill">
              {L(row.stage, stageZh[row.stage ?? ""])}
            </span>
          </td>
          <td>
            <AgentIcon name={row.owner ?? ""} />
            {row.owner}
          </td>
          <td>
            <b>${row.value}K</b>
          </td>
          <td>
            <Health value={row.health ?? 0} change={row.change} L={L} />
          </td>
          <td>
            <AgentIcon name={row.agent ?? ""} />
            {row.agent} · {relativeTime(row.touch, L)}
          </td>
          <td>
            <span className="ws-next-step">
              {L(row.next, row.nextZh)}
              {companyRowMeta[row.name]?.[0] && (
                <small>
                  {L(companyRowMeta[row.name][0], companyRowMeta[row.name][1])}
                </small>
              )}
            </span>
          </td>
          <td>
            <span className="ws-signal-cell">
              <span className="ws-signal" title={L(row.signal, row.signalZh)}>
                <i
                  className={signalTone(row.name)}
                  role="img"
                  aria-label={L(
                    signalTone(row.name) === "risk"
                      ? "Risk signal"
                      : signalTone(row.name) === "positive"
                        ? "Positive signal"
                        : "Signal",
                    signalTone(row.name) === "risk"
                      ? "风险信号"
                      : signalTone(row.name) === "positive"
                        ? "积极信号"
                        : "信号",
                  )}
                />
                <span>{L(row.signal, row.signalZh)}</span>
              </span>
              {companyRowMeta[row.name]?.[2] > 0 && (
                <span className="ws-signal-count">
                  +{companyRowMeta[row.name][2]}
                </span>
              )}
            </span>
          </td>
          <td>
            {L(
              row.segment,
              {
                Enterprise: "企业客户",
                "Mid-market": "中型客户",
                Startup: "初创企业",
              }[row.segment ?? ""],
            )}
          </td>
          <td className="ws-row-actions">
            <button
              title={L(
                `Delegate work on ${row.name}`,
                `委派 ${row.name} 的任务`,
              )}
              onClick={(event) => {
                event.stopPropagation();
                openDelegate();
              }}
            >
              <Plus size={14} />
            </button>
            <button
              title={L(`Open ${row.name}`, `打开 ${row.name}`)}
              onClick={(event) => {
                event.stopPropagation();
                navigate(row.name);
              }}
            >
              <OpenIcon size={14} />
            </button>
          </td>
        </>
      )}
    </tr>
  );
}
