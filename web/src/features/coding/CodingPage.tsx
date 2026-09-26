import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Bot, FolderGit2, GitBranch, Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useTasks } from "./api";
import NewTaskDialog from "./components/NewTaskDialog";
import StatusBadge from "./components/StatusBadge";
import { FILTERS, filterTasks, sortTasks, taskTitle, type Filter, type Task } from "./logic";

const FILTER_KEY = "xc.coding.filter";

function readFilter(): Filter {
  try {
    const v = localStorage.getItem(FILTER_KEY) as Filter | null;
    return v && FILTERS.some((f) => f.id === v) ? v : "all";
  } catch {
    return "all";
  }
}

function TaskRow({ task }: { task: Task }) {
  const t = useT();
  const language = useLanguage();
  const when = task.status === "running" && task.startedAt ? task.startedAt : task.finishedAt ?? task.createdAt;
  return (
    <Link to={`/coding/${task.id}`} className="coding-row">
      <StatusBadge status={task.status} />
      <div className="coding-row-main">
        <strong>{taskTitle(task)}</strong>
        <small>
          <span>
            <FolderGit2 size={12} /> {task.repoName}
          </span>
          <span className="xc-mono">
            <GitBranch size={12} /> {task.branch}
          </span>
          {task.issueKey && <span className="xc-badge">{task.issueKey}</span>}
          {task.status === "queued" && task.queuePosition && (
            <span>
              {t("Position in queue")}: {task.queuePosition}
            </span>
          )}
          {task.changedFiles.length > 0 && (
            <span>
              {task.changedFiles.length} {t("files changed")}
            </span>
          )}
        </small>
      </div>
      <div className="coding-row-side">
        <span className="coding-executor">{task.executor === "claude" ? "Claude Code" : "Codex"}</span>
        <small title={when}>{relativeTime(when, language)}</small>
      </div>
    </Link>
  );
}

export default function CodingPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const tasks = useTasks();
  const [filter, setFilterState] = useState<Filter>(readFilter);
  const setFilter = (f: Filter) => {
    setFilterState(f);
    try {
      localStorage.setItem(FILTER_KEY, f);
    } catch {
      /* 忽略 */
    }
  };
  const newOpen = params.get("new") === "1";
  const issueKey = params.get("issue") ?? undefined;
  const closeNew = () => {
    params.delete("new");
    params.delete("issue");
    setParams(params, { replace: true });
  };
  const visible = useMemo(() => sortTasks(filterTasks(tasks.data ?? [], filter)), [tasks.data, filter]);
  const counts = useMemo(() => {
    const out: Partial<Record<Filter, number>> = {};
    for (const f of FILTERS) out[f.id] = filterTasks(tasks.data ?? [], f.id).length;
    return out;
  }, [tasks.data]);

  return (
    <div className="xc-page coding-page">
      <PageHeading
        title={t("Coding tasks")}
        subtitle={t("Claude Code and Codex work on your repositories in separate worktrees.")}
        aside={
          <>
            <Link className="xc-btn" to="/coding/repos">
              <FolderGit2 size={14} /> {t("Repositories")}
            </Link>
            <button className="xc-btn primary" onClick={() => setParams({ new: "1" })}>
              <Plus size={14} /> {t("New task")}
            </button>
          </>
        }
      />
      <div className="xc-tabs" role="tablist">
        {FILTERS.map((f) => (
          <button
            key={f.id}
            role="tab"
            aria-selected={filter === f.id}
            className={filter === f.id ? "active" : ""}
            onClick={() => setFilter(f.id)}
          >
            {t(f.label)}
            {counts[f.id] ? <span className="coding-count">{counts[f.id]}</span> : null}
          </button>
        ))}
      </div>
      {tasks.isPending ? (
        <Loading />
      ) : tasks.isError ? (
        <ErrorState error={tasks.error} onRetry={() => tasks.refetch()} />
      ) : visible.length === 0 ? (
        <EmptyState title={filter === "all" ? t("No coding tasks yet") : t("Nothing here")} icon={<Bot size={28} />}>
          {filter === "all" && (
            <>
              <span>{t("Pick a repository, describe the change, and let the assistant work on it.")}</span>
              <button className="xc-btn primary" onClick={() => setParams({ new: "1" })}>
                <Plus size={14} /> {t("New task")}
              </button>
            </>
          )}
        </EmptyState>
      ) : (
        <div className="coding-list">
          {visible.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </div>
      )}
      <NewTaskDialog open={newOpen} onClose={closeNew} issueKey={issueKey} />
    </div>
  );
}
