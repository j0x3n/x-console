import type * as Model from "../../../types/domain";
import React from "react";
import AgentIcon from "../../../components/ui/AgentIcon";
import {
  MoreHorizontal,
  ArrowUpRight,
  Play,
  Pause,
  Activity,
  ArrowRight,
} from "lucide-react";
import { durationTime } from "../../../lib/formatters";

interface CrewAgentCardProps {
  name: string;
  index: number;
  charts: Record<string, Model.AgentChart>;
  L: Model.Localize;
  descriptions: Record<string, Model.Bilingual>;
  paused: Record<string, boolean>;
  pending: Model.Decision | undefined;
  setMenu: Model.Setter<string | null>;
  menu: string | null;
  openDelegate: Model.OpenDelegate;
  setPaused: Model.Setter<Record<string, boolean>>;
  setAgentFilter: Model.Setter<string>;
  stat: Model.AgentStats;
  navigate: Model.Navigate;
  mode: Model.AgentMode;
  modes: Record<string, Model.AgentMode>;
  setModes: Model.Setter<Record<string, Model.AgentMode>>;
  modeNotes: Record<string, Model.Bilingual>;
}

export default function CrewAgentCard({
  name,
  index,
  charts,
  L,
  descriptions,
  paused,
  pending,
  setMenu,
  menu,
  openDelegate,
  setPaused,
  setAgentFilter,
  stat,
  navigate,
  mode,
  modes,
  setModes,
  modeNotes,
}: CrewAgentCardProps) {
  return (
    <section
      className="ws-crew-card"
      key={name}
      id={name.toLowerCase()}
      style={{ "--index": index, "--crew-accent": charts[name].color }}
    >
      <div className="ws-crew-head">
        <AgentIcon name={name} />
        <div>
          <h2>{name}</h2>
          <p>{L(...descriptions[name])}</p>
        </div>
        <span
          className={
            paused[name]
              ? "ws-agent-state paused"
              : pending
                ? "ws-agent-state waiting"
                : "ws-agent-state running"
          }
        >
          {paused[name]
            ? L("Paused", "已暂停")
            : pending
              ? L("Needs your call", "待你决定")
              : L("Running", "运行中")}
        </span>
        <div className="ws-card-menu">
          <button
            onClick={() => setMenu(menu === name ? null : name)}
            title={L(`${name} actions`, `${name} 操作`)}
            aria-haspopup="menu"
            aria-expanded={menu === name}
          >
            <MoreHorizontal size={16} />
          </button>
          {menu === name && (
            <div role="menu" aria-label={L(`${name} actions`, `${name} 操作`)}>
              <button
                role="menuitem"
                onClick={() => {
                  setMenu(null);
                  openDelegate(name);
                }}
              >
                <ArrowUpRight size={13} />
                {L(`Delegate to ${name}…`, `委派给 ${name}…`)}
              </button>
              <button
                onClick={() => {
                  setPaused((prev) => ({
                    ...prev,
                    [name]: !prev[name],
                  }));
                  setMenu(null);
                }}
              >
                {paused[name] ? <Play size={13} /> : <Pause size={13} />}
                {paused[name]
                  ? L("Resume", "继续运行")
                  : L("Pause for today", "今日暂停")}
              </button>
              <button
                onClick={() => {
                  setAgentFilter(name);
                  document
                    .getElementById("recent-runs")
                    ?.scrollIntoView({ behavior: "smooth" });
                  setMenu(null);
                }}
              >
                <Activity size={13} />
                {L("View runs", "查看执行记录")}
              </button>
            </div>
          )}
        </div>
      </div>
      <div className="ws-agent-stats">
        <div>
          <small>
            {L("RUNS TODAY", "今日执行")}{" "}
            <em>
              {stat.overnight} {L("overnight", "夜间")}
            </em>
          </small>
          <strong>{stat.runs}</strong>
          <span className="ws-mini-bars">
            {stat.bars.map((height, i) => (
              <i key={i} style={{ height: `${Math.round(height * 0.47)}px` }} />
            ))}
            <span className="ws-week-labels">F S S M T W T</span>
          </span>
        </div>
        <div>
          <small>
            {L("SUCCESS", "成功率")} <em>{L("30 days", "近 30 天")}</em>
          </small>
          <strong className="ws-success">
            <i style={{ "--success": `${stat.success}%` }} />
            {stat.success}%
          </strong>
          <span className="ws-retried">
            {charts[name].retried} {L("retried", "次重试")}
          </span>
        </div>
        <div>
          <small>
            {L("SPEND", "花费")}{" "}
            <em>{L(`of $${stat.budget}`, `预算 $${stat.budget}`)}</em>
          </small>
          <strong>${stat.spend.toFixed(2)}</strong>
          <span
            className="ws-spend-ticks"
            role="img"
            aria-label={L(
              `$${stat.spend} of a $${stat.budget} budget`,
              `预算 $${stat.budget}，已花费 $${stat.spend}`,
            )}
          >
            {Array.from({ length: 10 }, (_, i) => (
              <i
                key={i}
                style={{
                  "--fill": `${Math.max(0, Math.min(100, ((stat.spend / stat.budget) * 10 - i) * 100))}%`,
                }}
              />
            ))}
          </span>
        </div>
        <div>
          <small>
            {L("MEDIAN RUN", "执行中位时长")} <em>{L("today", "今天")}</em>
          </small>
          <strong>{durationTime(stat.median, L)}</strong>
          <span
            className="ws-median-range"
            role="img"
            aria-label={L(
              `Median ${stat.median}`,
              `中位耗时 ${durationTime(stat.median, L)}`,
            )}
          >
            <i
              style={{
                left: `${(charts[name].range[0] / 240) * 100}%`,
                width: `${((charts[name].range[2] - charts[name].range[0]) / 240) * 100}%`,
              }}
            />
            <b style={{ left: `${(charts[name].range[1] / 240) * 100}%` }} />
            <span>
              0 <em>2m</em> 4m
            </span>
          </span>
        </div>
      </div>
      <button
        className="ws-current-run"
        onClick={() => navigate(pending ? "Today" : "Work")}
      >
        <small>{pending ? L("Your call", "待决定") : L("Now", "进行中")}</small>
        <span>{L(stat.current, stat.currentZh)}</span>
        <ArrowRight size={14} />
      </button>
      <div
        className="ws-autonomy"
        role="group"
        aria-label={L(`${name} autonomy`, `${name} 自主模式`)}
      >
        {(["Suggest only", "Ask first", "Autopilot"] as const).map((option) => (
          <button
            key={option}
            className={mode === option ? "active" : ""}
            onClick={() => {
              const next = { ...modes, [name]: option };
              setModes(next);
            }}
          >
            {L(
              option,
              {
                "Suggest only": "仅建议",
                "Ask first": "先询问",
                Autopilot: "自主执行",
              }[option],
            )}
          </button>
        ))}
      </div>
      <p className="ws-mode-note">{L(...modeNotes[mode])}</p>
    </section>
  );
}
