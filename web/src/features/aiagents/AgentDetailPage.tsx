import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { ListTodo, Pencil, Trash2 } from "lucide-react";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import Markdown from "../../components/markdown/Markdown";
import { useT } from "../../contexts/LanguageContext";
import { useRepos, useTasks } from "../coding/api";
import { TaskRow } from "../coding/CodingPage";
import { useAiAgent } from "./api";
import AgentAvatar from "./AgentAvatar";
import AgentDialog from "./AgentDialog";
import AgentRunLog from "./AgentRunLog";
import { useDeleteAgent } from "./useDeleteAgent";
import { costText, kindLabel } from "./logic";
import "./i18n";
import "./aiagents.css";

const ACCESS_LABEL: Record<string, string> = {
  read: "Read only",
  write: "Read and write",
  write_delete: "Read, write and delete",
};

/** /coding/agents/:id：一个 Agent 的设置、任务历史和费用（B47）。 */
export default function AgentDetailPage() {
  const t = useT();
  const id = Number(useParams().agentId);
  const agent = useAiAgent(id);
  const tasks = useTasks();
  const repos = useRepos();
  const [editing, setEditing] = useState(false);
  const navigate = useNavigate();
  const deleteAgent = useDeleteAgent();
  const mine = useMemo(
    () => (tasks.data ?? []).filter((x) => x.aiAgentId === id),
    [tasks.data, id],
  );
  if (agent.isPending) return <Loading />;
  if (agent.isError)
    return (
      <div className="xc-page">
        <ErrorState error={agent.error} onRetry={() => agent.refetch()} />
      </div>
    );
  const a = agent.data;
  const done = mine.filter((x) =>
    ["committed", "pushed", "pr_opened"].includes(x.status),
  ).length;
  const failed = mine.filter((x) => x.status === "failed").length;
  const repoNames = a.repoIds.map(
    (rid) =>
      repos.data?.find((r) => r.id === rid)?.remoteRepo ??
      repos.data?.find((r) => r.id === rid)?.name ??
      `#${rid}`,
  );

  return (
    <div className="xc-page">
      <PageHeading
        title={a.name}
        subtitle={`${t(kindLabel(a.kind))}${a.enabled ? "" : ` · ${t("Disabled")}`}`}
        aside={
          <>
            <button className="xc-btn" onClick={() => setEditing(true)}>
              <Pencil size={14} /> {t("Edit")}
            </button>
            {/* B86：删除，先二次确认，删完回到列表 */}
            <MoreMenu
              label={`${t("More")}: ${a.name}`}
              title={a.name}
              items={[
                {
                  key: "delete",
                  label: t("Delete agent"),
                  icon: <Trash2 size={14} />,
                  danger: true,
                  onSelect: async () => {
                    if (await deleteAgent(a)) navigate("/coding");
                  },
                },
              ]}
            />
          </>
        }
      />
      <StatStrip label={a.name}>
        <StatCard
          label={t("Working")}
          value={a.runningTasks}
          tone={a.runningTasks > 0 ? "accent" : undefined}
          foot={`${a.queuedTasks} ${t("queued")}`}
        />
        <StatCard
          label={t("Tasks")}
          value={mine.length}
          foot={`${done} ${t("done")}`}
        />
        <StatCard
          label={t("Failed")}
          value={failed}
          tone={failed > 0 ? "danger" : undefined}
        />
        <StatCard
          label={t("Cost this month")}
          value={costText(a)}
          tone={a.overBudget ? "danger" : undefined}
          foot={a.overBudget ? t("Over budget") : undefined}
        />
      </StatStrip>
      <div className="aiagent-detail">
        <main className="aiagent-detail-main">
          <section className="xc-card">
            <div className="xc-card-head">
              <h2>{t("Tasks")}</h2>
            </div>
            {tasks.isPending ? (
              <Loading />
            ) : mine.length === 0 ? (
              <EmptyState
                title={t("No tasks yet")}
                icon={<ListTodo size={24} />}
              >
                <span className="xc-muted">
                  {t("Assign a card to this agent from the card's page.")}
                </span>
              </EmptyState>
            ) : (
              <div className="coding-list">
                {mine.map((task) => (
                  <TaskRow key={task.id} task={task} />
                ))}
              </div>
            )}
          </section>
          <section className="xc-card aiagent-runlog-card">
            <div className="xc-card-head">
              <h2>{t("Execution log")}</h2>
            </div>
            <AgentRunLog agentId={a.id} hideWhenEmpty />
          </section>
        </main>
        <aside className="aiagent-detail-side">
          <div className="xc-card aiagent-props">
            <div className="aiagent-props-head">
              <AgentAvatar agent={a} size={40} />
              <strong>{a.name}</strong>
            </div>
            <dl>
              <dt>{t("Type")}</dt>
              <dd>{t(kindLabel(a.kind))}</dd>
              <dt>{t("Model")}</dt>
              <dd className="xc-mono">{a.model || t("Default")}</dd>
              {a.kind === "builtin" ? (
                <>
                  <dt>{t("Access")}</dt>
                  <dd>{t(ACCESS_LABEL[a.access] ?? a.access)}</dd>
                </>
              ) : (
                <>
                  <dt>{t("Default machine")}</dt>
                  <dd>{a.runnerName ?? t("The repository's machine")}</dd>
                  <dt>{t("Command line permission")}</dt>
                  <dd>
                    {a.cliPermission === "full" ? (
                      <span className="xc-badge warn">{t("Full")}</span>
                    ) : (
                      t("Work folder only")
                    )}
                  </dd>
                  <dt>{t("Repositories it may change")}</dt>
                  <dd>{repoNames.length ? repoNames.join("、") : t("None")}</dd>
                  <dt>{t("Build after each change")}</dt>
                  <dd>
                    {a.autoBuild
                      ? `${t("Yes")} · ${t("Retries after a failed build")} ${a.buildRetries}`
                      : t("No")}
                  </dd>
                </>
              )}
              <dt>{t("Tasks at the same time")}</dt>
              <dd>{a.maxParallel}</dd>
            </dl>
          </div>
          {a.instructions && (
            <div className="xc-card aiagent-props">
              <div className="xc-card-head">
                <h2>{t("Instructions")}</h2>
              </div>
              <Markdown source={a.instructions} />
            </div>
          )}
        </aside>
      </div>
      {editing && <AgentDialog agent={a} onClose={() => setEditing(false)} />}
    </div>
  );
}
