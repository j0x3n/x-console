import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import {
  ArrowLeft,
  CircleStop,
  ExternalLink,
  GitCommitHorizontal,
  GitPullRequest,
  RotateCcw,
  Trash2,
  Upload,
} from "lucide-react";
import { ApiError, errorMessage } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import Dialog from "../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useCancelTask,
  useCodingSettings,
  useCommitTask,
  useDiscardTask,
  useOpenPR,
  usePushTask,
  useRepos,
  useTask,
  useTaskDiff,
  useTaskEvents,
  type Task,
} from "./api";
import DiffView from "./components/DiffView";
import OutputView from "./components/OutputView";
import StatusBadge from "./components/StatusBadge";
import { actionsFor, formatDuration, isActive, taskTitle } from "./logic";

function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [active]);
  return now;
}

function CommitDialog({ task, open, onClose }: { task: Task; open: boolean; onClose: () => void }) {
  const t = useT();
  const commit = useCommitTask();
  const [message, setMessage] = useState("");
  useEffect(() => {
    if (open) setMessage("");
  }, [open]);
  const run = async (push: boolean) => {
    try {
      await withElevation(() => commit.mutateAsync({ id: task.id, message: message.trim(), push }));
      toast(push ? t("Committed and pushed") : t("Committed"));
      onClose();
    } catch (error) {
      if (error instanceof ApiError && error.code === "elevation_canceled") return;
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <Dialog open={open} onClose={onClose} title={t("Commit changes")}>
      <label className="xc-field">
        <span>{t("Commit message")}</span>
        <textarea
          className="xc-textarea"
          rows={4}
          value={message}
          placeholder={task.title}
          onChange={(e) => setMessage(e.target.value)}
        />
        <small>{t("Leave it empty to use the first line of the prompt.")}</small>
      </label>
      <div className="xc-dialog-actions">
        <button className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button className="xc-btn" disabled={commit.isPending} onClick={() => run(true)}>
          <Upload size={14} /> {t("Commit and push")}
        </button>
        <button className="xc-btn primary" disabled={commit.isPending} onClick={() => run(false)}>
          <GitCommitHorizontal size={14} /> {t("Commit")}
        </button>
      </div>
    </Dialog>
  );
}

function PRDialog({ task, open, onClose }: { task: Task; open: boolean; onClose: () => void }) {
  const t = useT();
  const pr = useOpenPR();
  const [title, setTitle] = useState("");
  const [draft, setDraft] = useState(false);
  useEffect(() => {
    if (open) {
      setTitle("");
      setDraft(false);
    }
  }, [open]);
  const run = async () => {
    try {
      await withElevation(() => pr.mutateAsync({ id: task.id, title: title.trim(), draft }));
      toast(t("Pull request opened"));
      onClose();
    } catch (error) {
      if (error instanceof ApiError && error.code === "elevation_canceled") return;
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <Dialog open={open} onClose={onClose} title={t("Open a pull request")}>
      <label className="xc-field">
        <span>{t("Title")}</span>
        <input className="xc-input" value={title} placeholder={task.title} onChange={(e) => setTitle(e.target.value)} />
      </label>
      <label className="coding-check">
        <input type="checkbox" checked={draft} onChange={(e) => setDraft(e.target.checked)} /> {t("Draft")}
      </label>
      <p className="xc-muted">
        {t("The branch is pushed first if needed.")} {task.branch} → {task.baseBranch}
      </p>
      <div className="xc-dialog-actions">
        <button className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button className="xc-btn primary" disabled={pr.isPending} onClick={run}>
          <GitPullRequest size={14} /> {t("Open pull request")}
        </button>
      </div>
    </Dialog>
  );
}

function Actions({ task }: { task: Task }) {
  const t = useT();
  const settings = useCodingSettings();
  const repos = useRepos();
  const cancel = useCancelTask();
  const push = usePushTask();
  const discard = useDiscardTask();
  const [commitOpen, setCommitOpen] = useState(false);
  const [prOpen, setPrOpen] = useState(false);
  const repo = repos.data?.find((r) => r.id === task.repoId);
  const prAvailable = !!settings.data?.prAvailable && !!repo?.githubRepo;
  const can = actionsFor(task, prAvailable);

  const guarded = async (fn: () => Promise<unknown>, done: string) => {
    try {
      await withElevation(fn);
      toast(done);
    } catch (error) {
      if (error instanceof ApiError && error.code === "elevation_canceled") return;
      toast({ message: errorMessage(error), tone: "error" });
    }
  };

  return (
    <div className="coding-actions">
      {can.cancel && (
        <button
          className="xc-btn danger"
          disabled={cancel.isPending}
          onClick={() => {
            if (task.status === "running" && !confirm(t("Stop this task? Its changes are thrown away."))) return;
            cancel.mutate(task.id, {
              onSuccess: () => toast(task.status === "running" ? t("Stopping...") : t("Canceled")),
              onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
            });
          }}
        >
          <CircleStop size={14} /> {task.status === "running" ? t("Stop") : t("Cancel task")}
        </button>
      )}
      {can.commit && (
        <button className="xc-btn primary" onClick={() => setCommitOpen(true)}>
          <GitCommitHorizontal size={14} /> {t("Commit")}
        </button>
      )}
      {can.push && (
        <button
          className="xc-btn"
          disabled={push.isPending}
          onClick={() => guarded(() => push.mutateAsync(task.id), t("Pushed"))}
        >
          <Upload size={14} /> {t("Push")}
        </button>
      )}
      {can.pr && (
        <button className="xc-btn primary" onClick={() => setPrOpen(true)}>
          <GitPullRequest size={14} /> {t("Open pull request")}
        </button>
      )}
      {!prAvailable && (task.status === "committed" || task.status === "pushed") && (
        <small className="xc-muted">
          {settings.data?.prAvailable ? t("The remote is not on GitHub.") : t("Connect GitHub to open pull requests.")}
        </small>
      )}
      {task.prUrl && (
        <a className="xc-btn" href={task.prUrl} target="_blank" rel="noreferrer">
          <ExternalLink size={14} /> {t("View pull request")}
        </a>
      )}
      {can.discard && (
        <button
          className="xc-btn ghost danger"
          disabled={discard.isPending}
          onClick={() => {
            if (!confirm(t("Discard this task? The worktree and the local branch are deleted."))) return;
            guarded(() => discard.mutateAsync(task.id), t("Discarded"));
          }}
        >
          <Trash2 size={14} /> {t("Discard")}
        </button>
      )}
      {!isActive(task.status) && (
        <Link
          className="xc-btn ghost"
          to={`/coding?new=1${task.issueKey ? `&issue=${encodeURIComponent(task.issueKey)}` : ""}`}
        >
          <RotateCcw size={14} /> {t("New task")}
        </Link>
      )}
      <CommitDialog task={task} open={commitOpen} onClose={() => setCommitOpen(false)} />
      <PRDialog task={task} open={prOpen} onClose={() => setPrOpen(false)} />
    </div>
  );
}

function Details({ task }: { task: Task }) {
  const t = useT();
  const language = useLanguage();
  const now = useNow(task.status === "running");
  const started = task.startedAt ? new Date(task.startedAt).getTime() : undefined;
  const ended = task.finishedAt ? new Date(task.finishedAt).getTime() : now;
  return (
    <dl className="coding-meta">
      <dt>{t("Repository")}</dt>
      <dd>{task.repoName}</dd>
      <dt>{t("Executor")}</dt>
      <dd>{task.executor === "claude" ? "Claude Code" : "Codex"}</dd>
      <dt>{t("Branch")}</dt>
      <dd className="xc-mono">{task.branch}</dd>
      <dt>{t("Base branch")}</dt>
      <dd className="xc-mono">
        {task.baseBranch || "HEAD"}
        {task.baseCommit && <span className="xc-muted"> @ {task.baseCommit.slice(0, 8)}</span>}
      </dd>
      {task.issueKey && (
        <>
          <dt>Issue</dt>
          <dd>
            <Link to={`/projects/${task.issueKey.split("-")[0]}/${task.issueKey.split("-")[1]}`}>{task.issueKey}</Link>
          </dd>
        </>
      )}
      {task.commitSha && (
        <>
          <dt>{t("Commit")}</dt>
          <dd className="xc-mono">{task.commitSha.slice(0, 12)}</dd>
        </>
      )}
      <dt>{t("Task created")}</dt>
      <dd title={task.createdAt}>{relativeTime(task.createdAt, language)}</dd>
      {started && (
        <>
          <dt>{t("Duration")}</dt>
          <dd>{formatDuration(ended - started)}</dd>
        </>
      )}
      <dt>{t("Time limit")}</dt>
      <dd>
        {task.timeoutMinutes} {t("min")}
      </dd>
    </dl>
  );
}

export default function TaskPage() {
  const t = useT();
  const id = Number(useParams().taskId);
  const task = useTask(id);
  const events = useTaskEvents(id);
  const status = task.data?.status;
  const can = task.data ? actionsFor(task.data, false) : undefined;
  const diff = useTaskDiff(id, status, !!can?.diff);
  const [promptOpen, setPromptOpen] = useState(false);

  if (task.isPending) return <Loading />;
  if (task.isError) {
    return (
      <div className="xc-page">
        <ErrorState error={task.error} onRetry={() => task.refetch()} />
      </div>
    );
  }
  const data = task.data;
  return (
    <div className="xc-page coding-page coding-task-page">
      <div className="coding-task-head">
        <Link to="/coding" className="xc-btn ghost small">
          <ArrowLeft size={14} /> {t("Coding tasks")}
        </Link>
        <div className="coding-task-title">
          <h1>{taskTitle(data)}</h1>
          <StatusBadge status={data.status} />
          {data.status === "queued" && data.queuePosition && (
            <span className="xc-muted">
              {t("Position in queue")}: {data.queuePosition}
            </span>
          )}
        </div>
      </div>
      {data.error && <p className={`coding-banner ${data.status === "canceled" ? "" : "danger"}`}>{data.error}</p>}
      <div className="coding-task-grid">
        <section className="xc-card coding-output-card">
          <div className="xc-card-head">
            <h2>{t("Output")}</h2>
            <button className="xc-btn ghost small" onClick={() => setPromptOpen(true)}>
              {t("Prompt")}
            </button>
          </div>
          {events.isPending ? (
            <Loading />
          ) : events.isError ? (
            <ErrorState error={events.error} onRetry={() => events.refetch()} />
          ) : (
            <OutputView events={events.data} running={isActive(data.status)} />
          )}
        </section>
        <aside className="coding-side">
          <section className="xc-card">
            <Actions task={data} />
          </section>
          <section className="xc-card">
            <Details task={data} />
          </section>
        </aside>
      </div>
      {can?.diff && (
        <section className="xc-card coding-changes">
          <div className="xc-card-head">
            <h2>{t("Changes")}</h2>
            {diff.data && (
              <span className="xc-muted">
                {diff.data.files.length} {t("files changed")}
              </span>
            )}
          </div>
          {diff.isPending ? (
            <Loading />
          ) : diff.isError ? (
            <EmptyState title={t("Changes are not available")}>
              <span>{errorMessage(diff.error)}</span>
              <button className="xc-btn small" onClick={() => diff.refetch()}>
                {t("Retry")}
              </button>
            </EmptyState>
          ) : (
            <DiffView diff={diff.data} />
          )}
        </section>
      )}
      <Dialog open={promptOpen} onClose={() => setPromptOpen(false)} title={t("Prompt")} wide>
        <pre className="coding-prompt-view">{data.prompt}</pre>
        <div className="xc-dialog-actions">
          <button className="xc-btn primary" onClick={() => setPromptOpen(false)}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    </div>
  );
}
