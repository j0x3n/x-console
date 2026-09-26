import type * as Model from "../../types/domain";
import React, { useState, useEffect } from "react";
import { agentStats } from "../../data/workspace";
import { hireTemplates } from "./hire-config";
import { crewRuns } from "../../data/crew-runs";
import PageHeading from "../../components/ui/PageHeading";
import CrewAgentCard from "./components/CrewAgentCard";
import { HiredAgentCard } from "./components/HiredAgentCard";
import HireAgentTile from "./components/HireAgentTile";
import RecentRuns from "./components/RecentRuns";
import { HireAgentDialog } from "./components/HireAgentDialog";
import { usePreferencesStore } from "../../stores/preferences-store";

interface CrewPageProps {
  L: Model.Localize;
  language: Model.Language;
  navigate: Model.Navigate;
  openDelegate: Model.OpenDelegate;
  activity: Model.ActivityRow[];
  decisions: Model.Decision[];
  hireRequest: number;
  onHireRequestHandled: () => void;
  hired: Model.HiredAgent[];
  setHired: Model.Setter<Model.HiredAgent[]>;
  onNotify: Model.Notify;
}

export default function CrewPage({
  L,
  language,
  navigate,
  openDelegate,
  activity,
  decisions,
  hireRequest,
  onHireRequestHandled,
  hired,
  setHired,
  onNotify,
}: CrewPageProps) {
  const modes = usePreferencesStore((state) => state.agentModes);
  const setModes = usePreferencesStore((state) => state.setAgentModes);
  const [paused, setPaused] = useState<Record<string, boolean>>({});
  const [menu, setMenu] = useState<string | null>(null);
  const [agentFilter, setAgentFilter] = useState("All");
  const [showAll, setShowAll] = useState(false);
  const [hireOpen, setHireOpen] = useState(false);
  useEffect(() => {
    if (!menu) return;
    const closeOutside = (event: PointerEvent) => {
      if (
        event.target instanceof Element &&
        !event.target.closest(".ws-card-menu")
      )
        setMenu(null);
    };
    const closeEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenu(null);
    };
    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("keydown", closeEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("keydown", closeEscape);
    };
  }, [menu]);
  useEffect(() => {
    if (hireRequest > 0) {
      setHireOpen(true);
      onHireRequestHandled();
    }
  }, [hireRequest]);
  const agentNames = Object.keys(agentStats);
  const charts: Record<string, Model.AgentChart> = {
    Scout: { color: "#4b9e8c", retried: 38, range: [74, 112, 158] },
    Scribe: { color: "#b89439", retried: 31, range: [31, 48, 70] },
    Ledger: { color: "#7b61dc", retried: 0, range: [46, 70, 101] },
    Pilot: { color: "#799f3b", retried: 10, range: [12, 22, 33] },
    Echo: { color: "#cf527a", retried: 28, range: [98, 155, 212] },
  };
  const descriptions: Record<string, Model.Bilingual> = {
    Scout: [
      "Finds buying signals and researches accounts",
      "寻找采购信号并研究客户",
    ],
    Scribe: [
      "Writes call notes, recaps and follow-ups",
      "整理通话记录、纪要和跟进邮件",
    ],
    Ledger: [
      "Watches renewals, usage and billing risk",
      "关注续约、使用量和账单风险",
    ],
    Pilot: [
      "Keeps stages, dates and next steps current",
      "维护商机阶段、日期和下一步",
    ],
    Echo: [
      "Preps every meeting and briefs you 15 min before",
      "准备每场会议并提前 15 分钟提供简报",
    ],
  };
  const modeNotes: Record<string, Model.Bilingual> = {
    "Suggest only": [
      "Creates suggestions for you to review.",
      "仅生成建议，等待你查看。",
    ],
    "Ask first": [
      "Waits for your approval before taking action.",
      "执行前会等待你的批准。",
    ],
    Autopilot: [
      "Updates routine records independently; receipts stay visible.",
      "自主更新日常记录，执行依据始终可查。",
    ],
  };
  const availableTemplates = hireTemplates.filter(
    (template) => !hired.some((agent) => agent.template === template.id),
  );
  const newRuns = activity
    .filter(
      (row) =>
        !crewRuns.some((run) => run.agent === row[0] && run.run === row[1]),
    )
    .map((row, index) => ({
      id: `new-${index}`,
      time: L(row[4]),
      agent: row[0],
      run: row[1],
      record: row[2],
      receipt: row[3],
      took: "—",
      cost: null,
      offline: false,
    }));
  const recentRuns = [...newRuns, ...crewRuns];
  const filteredActivity = recentRuns.filter(
    (row) => agentFilter === "All" || row.agent === agentFilter,
  );
  const visibleRuns = filteredActivity.slice(0, showAll ? undefined : 12);
  const firstOffline = visibleRuns.findIndex((row) => row.offline);
  return (
    <div className="workspace-page ws-crew-page">
      <PageHeading
        title={L(
          "142 runs today, $18.40 of $30",
          "今日执行 142 次，已花费 $18.40 / $30",
        )}
        subtitle={L(
          `${5 + hired.length} agents, 37 runs while you were offline. ${decisions.length} need your call.`,
          `${5 + hired.length} 位智能助手，你离线期间执行 37 次。${decisions.length} 项等待你决定。`,
        )}
        aside={
          <span className="ws-running-summary">
            <i />
            {L(
              `${agentNames.filter((name) => ["Scout", "Echo"].includes(name) && !paused[name]).length} agents running now`,
              `${agentNames.filter((name) => ["Scout", "Echo"].includes(name) && !paused[name]).length} 位助手正在运行`,
            )}
          </span>
        }
      />
      <div className="ws-crew-grid">
        {agentNames.map((name, index) => {
          const stat = agentStats[name];
          const mode = modes[name] || stat.mode;
          const pending = decisions.find((item) => item.agent === name);
          return (
            <CrewAgentCard
              key={name}
              name={name}
              index={index}
              charts={charts}
              L={L}
              descriptions={descriptions}
              paused={paused}
              pending={pending}
              setMenu={setMenu}
              menu={menu}
              openDelegate={openDelegate}
              setPaused={setPaused}
              setAgentFilter={setAgentFilter}
              stat={stat}
              navigate={navigate}
              mode={mode}
              modes={modes}
              setModes={setModes}
              modeNotes={modeNotes}
            />
          );
        })}
        {hired.map((agent, position) => (
          <HiredAgentCard
            key={agent.id}
            agent={agent}
            language={language}
            onChange={(next) =>
              setHired((current) =>
                current.map((item) => (item.id === next.id ? next : item)),
              )
            }
            onRemove={() => {
              setHired((current) =>
                current.filter((item) => item.id !== agent.id),
              );
              onNotify({
                message: L(
                  `${agent.name} left the crew`,
                  `${agent.name} 已离开智能团队`,
                ),
                subtitle: L(
                  "No runs were made, so nothing was charged.",
                  "尚未执行任务，因此没有产生费用。",
                ),
                onUndo: () =>
                  setHired((current) => {
                    if (current.some((item) => item.id === agent.id))
                      return current;
                    const restored = [...current];
                    restored.splice(position, 0, agent);
                    return restored;
                  }),
              });
            }}
          />
        ))}
        <HireAgentTile
          setHireOpen={setHireOpen}
          availableTemplates={availableTemplates}
          L={L}
        />
      </div>
      <RecentRuns
        L={L}
        recentRuns={recentRuns}
        agentFilter={agentFilter}
        setAgentFilter={setAgentFilter}
        agentNames={agentNames}
        visibleRuns={visibleRuns}
        firstOffline={firstOffline}
        navigate={navigate}
        decisions={decisions}
        filteredActivity={filteredActivity}
        setShowAll={setShowAll}
        showAll={showAll}
      />
      {hireOpen && (
        <HireAgentDialog
          language={language}
          hired={hired}
          onClose={() => setHireOpen(false)}
          onHire={(agent) => {
            setHired((current) => [...current, agent]);
            setHireOpen(false);
            onNotify({
              message: L(
                `${agent.name} joined the crew`,
                `${agent.name} 已加入智能团队`,
              ),
              subtitle: L(
                `First run at 9:30 AM · ${agent.mode} · $${agent.budget} a day`,
                `上午 9:30 首次执行 · ${agent.mode === "Ask first" ? "先询问" : "仅建议"} · 每天 $${agent.budget}`,
              ),
            });
          }}
        />
      )}
    </div>
  );
}
