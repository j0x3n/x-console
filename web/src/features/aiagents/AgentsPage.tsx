import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Bot, Pencil, Plus, Trash2 } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import Switch from "../../components/ui/Switch";
import MoreMenu from "../../components/ui/MoreMenu";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { Toolbar } from "../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useT } from "../../contexts/LanguageContext";
import { useTasks } from "../coding/api";
import { useAgentMutations, useAiAgents, type AiAgent } from "./api";
import AgentAvatar from "./AgentAvatar";
import AgentDialog from "./AgentDialog";
import AgentTabs from "./AgentTabs";
import { costText, kindLabel } from "./logic";
import "./i18n";
import "./aiagents.css";

/** /coding：Agent 管理（B47）。 */
export default function AgentsPage() {
  const t = useT();
  const agents = useAiAgents();
  const tasks = useTasks();
  const ops = useAgentMutations();
  const [params, setParams] = useSearchParams();
  const [editing, setEditingState] = useState<AiAgent | "new" | null>(() =>
    params.get("new") === "1" ? "new" : null,
  );
  const setEditing = (v: AiAgent | "new" | null) => {
    setEditingState(v);
    if (!v && params.get("new")) setParams({}, { replace: true });
  };

  const heading = (
    <PageHeading
      title={t("Agents")}
      subtitle={t(
        "Agents change code on your machines, or work on cards with the console's tools.",
      )}
      aside={
        <button className="xc-btn primary" onClick={() => setEditing("new")}>
          <Plus size={14} /> {t("New agent")}
        </button>
      }
    />
  );
  const dialog = editing && (
    <AgentDialog
      agent={editing === "new" ? undefined : editing}
      onClose={() => setEditing(null)}
    />
  );
  if (agents.isPending)
    return (
      <div className="xc-page">
        {heading}
        <Loading />
      </div>
    );
  if (agents.isError)
    return (
      <div className="xc-page">
        {heading}
        <ErrorState error={agents.error} onRetry={() => agents.refetch()} />
      </div>
    );

  const list = agents.data;
  const running = list.reduce((n, a) => n + a.runningTasks, 0);
  const queued = list.reduce((n, a) => n + a.queuedTasks, 0);
  const review = (tasks.data ?? []).filter(
    (x) => x.status === "review" && x.aiAgentId != null,
  ).length;
  const cost = list.reduce((n, a) => n + a.monthCostUsd, 0);

  return (
    <div className="xc-page aiagents-page">
      {heading}
      <Toolbar start={<AgentTabs />} />
      <StatStrip label={t("Agents")}>
        <StatCard
          label={t("Agents")}
          value={list.length}
          foot={`${list.filter((a) => a.enabled).length} ${t("agents on")}`}
        />
        <StatCard
          label={t("Working")}
          value={running}
          tone={running > 0 ? "accent" : undefined}
          foot={`${queued} ${t("queued")}`}
        />
        <StatCard
          label={t("Waiting for review")}
          value={review}
          tone={review > 0 ? "warn" : undefined}
          to="/coding/tasks?filter=review"
        />
        <StatCard label={t("Cost this month")} value={`$${cost.toFixed(2)}`} />
      </StatStrip>
      {list.length === 0 ? (
        <EmptyState title={t("No agents yet")} icon={<Bot size={26} />}>
          <span className="xc-muted">
            {t(
              "For example: a backend developer that only changes the backend folder and runs the tests before it is done.",
            )}
          </span>
          <button className="xc-btn small" onClick={() => setEditing("new")}>
            <Plus size={14} /> {t("New agent")}
          </button>
        </EmptyState>
      ) : (
        <div className="aiagents-grid">
          {list.map((a) => (
            <AgentCard
              key={a.id}
              agent={a}
              onEdit={() => setEditing(a)}
              onDelete={async () => {
                if (
                  await confirmAction({
                    title: `${t("Delete agent")}“${a.name}”？`,
                    description: t(
                      "Its finished tasks stay. Cards keep it as a member until you remove it.",
                    ),
                    confirmLabel: t("Delete"),
                  })
                )
                  ops.remove.mutate(a.id);
              }}
              onToggle={(enabled) =>
                ops.update.mutate({ id: a.id, body: { enabled } })
              }
            />
          ))}
        </div>
      )}
      {dialog}
    </div>
  );
}

function AgentCard({
  agent: a,
  onEdit,
  onDelete,
  onToggle,
}: {
  agent: AiAgent;
  onEdit: () => void;
  onDelete: () => void;
  onToggle: (enabled: boolean) => void;
}) {
  const t = useT();
  return (
    <div className={`xc-card aiagent-card${a.enabled ? "" : " off"}`}>
      <div className="aiagent-card-head">
        <AgentAvatar agent={a} size={34} />
        <Link to={`/coding/agents/${a.id}`} className="aiagent-card-name">
          <strong>{a.name}</strong>
          <small>
            {t(kindLabel(a.kind))}
            {a.runnerName ? ` · ${a.runnerName}` : ""}
          </small>
        </Link>
        <Switch
          checked={a.enabled}
          label={`${a.name} ${t("enabled")}`}
          onChange={onToggle}
        />
        <MoreMenu
          label={`${t("More")}: ${a.name}`}
          title={a.name}
          items={[
            {
              key: "edit",
              label: t("Edit"),
              icon: <Pencil size={14} />,
              onSelect: onEdit,
            },
            {
              key: "delete",
              label: t("Delete"),
              icon: <Trash2 size={14} />,
              danger: true,
              onSelect: onDelete,
            },
          ]}
        />
      </div>
      {a.instructions && <p className="aiagent-card-brief">{a.instructions}</p>}
      <div className="aiagent-card-foot">
        {a.runningTasks > 0 ? (
          <span className="xc-badge accent">
            {t("Working")} {a.runningTasks}
          </span>
        ) : (
          <span className="xc-badge">{t("Idle")}</span>
        )}
        {a.queuedTasks > 0 && (
          <span className="xc-badge">
            {a.queuedTasks} {t("queued")}
          </span>
        )}
        {a.overBudget && (
          <span className="xc-badge danger">{t("Over budget")}</span>
        )}
        {a.kind !== "builtin" && (
          <span className="xc-muted">
            {a.repoIds.length} {t("repositories")}
          </span>
        )}
        <span className="aiagent-card-cost" title={t("Cost this month")}>
          {costText(a)}
        </span>
      </div>
    </div>
  );
}
