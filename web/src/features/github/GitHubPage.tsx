import { useMemo, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import {
  AlertTriangle,
  Bell,
  BellOff,
  Bot,
  ChevronRight,
  CircleCheck,
  CircleDashed,
  CircleDot,
  CircleMinus,
  CircleX,
  ExternalLink,
  GitBranch,
  GitCommitHorizontal,
  GitPullRequest,
  Github,
  Layers,
  Loader,
  Lock,
  RefreshCw,
  Settings,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  isNotConfigured,
  useCommits,
  useGitHubIssues,
  useGitHubStatus,
  usePulls,
  useRunJobs,
  useRuns,
  useSyncGitHub,
  type CheckState,
  type GitHubCommit,
  type GitHubIssue,
  type GitHubJob,
  type GitHubPull,
  type GitHubRun,
  type GitHubStatus,
} from "./api";
import {
  checkLabel,
  checkTone,
  duration,
  issuePath,
  jobsProgress,
  latestDefaultRuns,
  parseRepoKey,
  repoKey,
  reviewLabel,
  reviewTone,
  runOutcome,
  sameRepo,
  sortPulls,
  splitRepo,
  stepState,
  type RepoRow,
} from "./logic";
import { useRepoList } from "./useRepoList";
import NotifyDialog from "./NotifyDialog";
import { ForgeIcon, RepoSwatch } from "./RepoBits";

type Tab = "pulls" | "commits" | "runs" | "issues";
const tabs: Tab[] = ["pulls", "commits", "runs", "issues"];

/** 选中的仓库。null 表示“全部仓库”。 */
type Selection = { connectionId: number; repo: string } | null;

/** 仓库多于这个数时，左栏按 owner 分组。 */

export default function GitHubPage() {
  const t = useT();
  const status = useGitHubStatus();
  const [params, setParams] = useSearchParams();
  const tab: Tab = tabs.includes(params.get("tab") as Tab)
    ? (params.get("tab") as Tab)
    : "pulls";
  const repoParam = params.get("repo") ?? "";
  const sel: Selection = repoParam ? parseRepoKey(repoParam) : null;
  const setView = (next: { tab?: Tab; repo?: string | null }) =>
    setParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        const nextTab = next.tab ?? tab;
        if (nextTab === "pulls") p.delete("tab");
        else p.set("tab", nextTab);
        if (next.repo !== undefined) {
          if (next.repo) p.set("repo", next.repo);
          else p.delete("repo");
        }
        return p;
      },
      { replace: true },
    );

  const configured = status.data?.configured === true;
  let content;
  if (status.isPending) content = <Loading />;
  else if (status.isError)
    content = (
      <ErrorState error={status.error} onRetry={() => status.refetch()} />
    );
  else if (!configured) content = <SetupGuide />;
  else
    content = (
      <>
        {status.data.lastError && <SyncError message={status.data.lastError} />}
        {status.data.repoCount === 0 && (
          <div className="xc-card github-notice" role="status">
            <span>
              {t("No watched repositories yet. Pick some in settings.")}
            </span>
            <Link className="xc-btn small" to="/settings/git">
              {t("Settings")}
            </Link>
          </div>
        )}
        <GitHubStats status={status.data} />
        <ReposBody
          sel={sel}
          tab={tab}
          onTab={(next) => setView({ tab: next })}
        />
      </>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Repositories")}
        subtitle={
          status.data?.configured
            ? `${status.data.login ? `@${status.data.login} · ` : ""}${status.data.repoCount} ${t("repositories watched")}`
            : undefined
        }
        aside={status.data?.configured && <SyncControl status={status.data} />}
      />
      {content}
    </div>
  );
}

function ReposBody({
  sel,
  tab,
  onTab,
}: {
  sel: Selection;
  tab: Tab;
  onTab: (tab: Tab) => void;
}) {
  const t = useT();
  const list = useRepoList();
  const current = sel
    ? (list.rows.find((r) => sameRepo(r, sel)) ?? null)
    : null;
  const pulls = usePulls();
  const issues = useGitHubIssues();
  const openPulls = (pulls.data ?? []).filter(
    (p) => p.state === "open" && (!sel || sameRepo(p, sel)),
  ).length;
  const openIssues = (issues.data ?? []).filter(
    (i) => !sel || sameRepo(i, sel),
  ).length;
  return (
    // B103：仓库列表在左栏二级菜单里，页面不再放一份
    <div className="repos-layout">
      <section className="repos-main">
        <RepoHeader sel={sel} row={current} total={list.rows.length} />
        <nav className="xc-tabs repos-tabs">
          {(
            [
              ["pulls", t("Pull requests"), openPulls],
              ["commits", t("Commits"), null],
              ["runs", t("CI runs"), null],
              ["issues", t("Issues"), openIssues],
            ] as const
          ).map(([key, label, count]) => (
            <button
              key={key}
              className={tab === key ? "active" : ""}
              onClick={() => onTab(key)}
            >
              {label}
              {count ? (
                <small className="repos-tab-count">{count}</small>
              ) : null}
            </button>
          ))}
        </nav>
        {tab === "pulls" && <PullsView sel={sel} />}
        {tab === "commits" && <CommitsView sel={sel} />}
        {tab === "runs" && <RunsView sel={sel} />}
        {tab === "issues" && <IssuesView sel={sel} />}
      </section>
    </div>
  );
}

/** 一个仓库的小标题：颜色块 + owner/name。“全部仓库”时每组的标题用它。 */
function RepoHead({
  repo,
  count,
  children,
}: {
  repo: string;
  count?: number;
  children?: ReactNode;
}) {
  const { owner, name } = splitRepo(repo);
  return (
    <h2 className="repos-group-title">
      <RepoSwatch repo={repo} />
      <span className="xc-mono">
        <span className="xc-muted">{owner}/</span>
        {name}
      </span>
      {count != null && <span className="xc-badge">{count}</span>}
      {children}
    </h2>
  );
}

/* ---- 右边的标题行 ---- */

function RepoHeader({
  sel,
  row,
  total,
}: {
  sel: Selection;
  row: RepoRow | null;
  total: number;
}) {
  const t = useT();
  const language = useLanguage();
  const [notifyOpen, setNotifyOpen] = useState(false);
  if (!sel)
    return (
      <header className="repos-head">
        <Layers size={16} className="xc-muted" />
        <strong>{t("All repositories")}</strong>
        <span className="xc-muted">
          {total} {t("repositories")}
        </span>
      </header>
    );
  const repo = row?.repo ?? sel.repo;
  const { owner, name } = splitRepo(repo);
  return (
    <header className="repos-head">
      <RepoSwatch repo={repo} />
      <strong className="repos-head-name">
        <span className="xc-muted">{owner}/</span>
        {name}
      </strong>
      {row?.private && (
        <Lock size={12} className="xc-muted" aria-label={t("Private")} />
      )}
      <span className="xc-badge repos-head-account">
        <ForgeIcon forge={row?.forge} size={11} />
        {row?.connectionName ??
          (row?.forge === "forgejo" ? "Forgejo" : "GitHub")}
      </span>
      {row?.defaultBranch && (
        <span className="xc-mono xc-muted repos-head-branch">
          <GitBranch size={12} /> {row.defaultBranch}
        </span>
      )}
      {row?.pushedAt && (
        <span className="xc-muted repos-head-time">
          {t("Last push")} {relativeTime(row.pushedAt, language)}
        </span>
      )}
      <span className="xc-spacer" />
      {row && (
        <button
          type="button"
          className={`xc-btn small ghost repos-bell${row.notifyCustom ? " custom" : ""}`}
          title={
            row.notifyOff
              ? t("Notifications are off for this repository")
              : row.notifyCustom
                ? t("This repository has its own notification settings")
                : t("Notifications")
          }
          aria-label={t("Notifications")}
          onClick={() => setNotifyOpen(true)}
        >
          {row.notifyOff ? <BellOff size={14} /> : <Bell size={14} />}
        </button>
      )}
      {row?.url && (
        <a
          className="xc-btn small ghost"
          href={row.url}
          target="_blank"
          rel="noreferrer"
          title={t("Open in browser")}
        >
          <ExternalLink size={14} />
          <span className="repos-btn-text">{t("Open")}</span>
        </a>
      )}
      {row?.syncError && (
        <p className="xc-error-text repos-head-error">{row.syncError}</p>
      )}
      {notifyOpen && row && (
        <NotifyDialog row={row} onClose={() => setNotifyOpen(false)} />
      )}
    </header>
  );
}

/* ---- 概要 ---- */

function GitHubStats({ status }: { status: GitHubStatus }) {
  const t = useT();
  const pulls = (usePulls().data ?? []).filter((p) => p.state === "open");
  const runs = useRuns().data ?? [];
  const issues = useGitHubIssues().data ?? [];
  const latest = latestDefaultRuns(runs);
  const failing = latest.filter((r) => runOutcome(r).tone === "danger").length;
  const passing = latest.filter((r) => runOutcome(r).tone === "ok").length;
  const approved = pulls.filter((p) => p.reviewState === "approved").length;
  const waiting = pulls.filter(
    (p) => p.reviewState === "pending" || p.reviewState === "none",
  ).length;
  return (
    <StatStrip label={t("Repositories")}>
      <StatCard
        label={t("Pull requests")}
        value={pulls.length}
        foot={`${approved} ${t("approved")} · ${waiting} ${t("waiting for review")}`}
      >
        <Segments
          parts={[
            { value: approved, tone: "ok" },
            { value: pulls.length - approved - waiting, tone: "warn" },
            { value: waiting, tone: "muted" },
          ]}
        />
      </StatCard>
      <StatCard
        label={t("CI on default branch")}
        value={failing}
        unit={t("failing")}
        tone={failing ? "danger" : latest.length ? "ok" : undefined}
        foot={`${passing} ${t("passing")} · ${latest.length} ${t("workflows")}`}
      />
      <StatCard
        label={t("Issues")}
        value={issues.length}
        foot={t("Open issues in watched repositories")}
      />
      <StatCard
        label={t("API quota")}
        value={status.rateLimitRemaining ?? "–"}
        foot={t("Requests left this hour")}
      />
    </StatStrip>
  );
}

function SyncControl({ status }: { status: GitHubStatus }) {
  const t = useT();
  const language = useLanguage();
  const sync = useSyncGitHub();
  const busy = sync.isPending || status.syncing;
  const onSync = () =>
    sync.mutate(undefined, {
      onSuccess: (s) =>
        s.lastError
          ? toast({ message: s.lastError, tone: "error" })
          : toast(t("Synced")),
      onError: (error) =>
        toast({ message: errorMessage(error), tone: "error" }),
    });
  return (
    <>
      {status.lastSyncAt && (
        <span className="xc-muted github-synced">
          {t("Synced")} {relativeTime(status.lastSyncAt, language)}
        </span>
      )}
      <button
        className="xc-btn small"
        onClick={onSync}
        disabled={busy}
        title={t("Sync now")}
      >
        <RefreshCw size={14} className={busy ? "github-spin" : undefined} />
        {busy ? t("Syncing") : t("Sync now")}
      </button>
    </>
  );
}

function SyncError({ message }: { message: string }) {
  const t = useT();
  return (
    <div className="xc-card github-notice warn" role="status">
      <AlertTriangle size={16} aria-hidden />
      <div>
        <strong>{t("Last sync had problems")}</strong>
        <p className="xc-muted">{message}</p>
      </div>
    </div>
  );
}

function SetupGuide() {
  const t = useT();
  return (
    <EmptyState title={t("Connect a Git account")} icon={<Github size={28} />}>
      <span>
        在设置里添加 GitHub 或 Forgejo 账号，选好要关注的仓库，就能在这里看
        PR、提交、CI 和 Issue。
      </span>
      <Link className="xc-btn small primary" to="/settings/git">
        <Settings size={14} /> {t("Go to settings")}
      </Link>
    </EmptyState>
  );
}

function CheckIcon({ state }: { state: CheckState }) {
  const t = useT();
  const label = t(checkLabel[state]);
  const props = {
    size: 15,
    "aria-label": label,
    className: `github-check ${checkTone(state)}`,
  };
  switch (state) {
    case "success":
      return <CircleCheck {...props} />;
    case "failure":
      return <CircleX {...props} />;
    case "pending":
      return <CircleDot {...props} />;
    default:
      return <CircleDashed {...props} />;
  }
}

function ListState({
  query,
  empty,
}: {
  query: {
    isPending: boolean;
    isError: boolean;
    error: unknown;
    refetch: () => unknown;
  };
  empty: string;
}) {
  const t = useT();
  if (query.isPending) return <Loading />;
  if (query.isError)
    return isNotConfigured(query.error) ? (
      <SetupGuide />
    ) : (
      <ErrorState error={query.error} onRetry={() => query.refetch()} />
    );
  return <EmptyState title={t(empty)} icon={<GitPullRequest size={28} />} />;
}

/** 按仓库分组的列表。选了一个仓库时只有一组，不显示组标题。 */
function Grouped<T extends { repo: string; connectionId?: number }>({
  items,
  sel,
  render,
}: {
  items: T[];
  sel: Selection;
  render: (item: T) => ReactNode;
}) {
  const groups = useMemo(() => {
    const map = new Map<string, T[]>();
    for (const item of items) {
      const k = repoKey(item.connectionId, item.repo);
      const list = map.get(k);
      if (list) list.push(item);
      else map.set(k, [item]);
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [items]);
  if (sel)
    return (
      <section className="xc-card repos-card">
        <ul className="github-list">{items.map(render)}</ul>
      </section>
    );
  return (
    <div className="xc-stack">
      {groups.map(([k, list]) => (
        <section className="xc-card repos-card" key={k}>
          <RepoHead repo={list[0].repo} count={list.length} />
          <ul className="github-list">{list.map(render)}</ul>
        </section>
      ))}
    </div>
  );
}

/* ---- PR ---- */

function PullsView({ sel }: { sel: Selection }) {
  const pulls = usePulls();
  const items = sortPulls(
    (pulls.data ?? []).filter((p) => !sel || sameRepo(p, sel)),
  );
  if (items.length === 0)
    return (
      <ListState
        query={{ ...pulls, isPending: pulls.isPending }}
        empty="No recent pull requests"
      />
    );
  return (
    <Grouped
      items={items}
      sel={sel}
      render={(p) => <PullRow key={`${p.repo}#${p.number}`} pull={p} />}
    />
  );
}

function PullRow({ pull }: { pull: GitHubPull }) {
  const t = useT();
  const language = useLanguage();
  return (
    <li className="github-item">
      <CheckIcon state={pull.checkState} />
      <div className="github-item-main">
        <a
          className="github-title"
          href={pull.url}
          target="_blank"
          rel="noreferrer"
        >
          {pull.title} <span className="xc-muted">#{pull.number}</span>
        </a>
        <div className="github-meta">
          <span>{pull.author}</span>
          <span
            className="xc-mono github-branch"
            title={`${pull.headRef} → ${pull.baseRef}`}
          >
            {pull.headRef} → {pull.baseRef}
          </span>
          <span>{relativeTime(pull.updatedAt, language)}</span>
        </div>
        <div className="github-badges">
          {pull.state !== "open" && (
            <span className="xc-badge info">
              {t(pull.state === "merged" ? "Merged" : "Closed")}
            </span>
          )}
          {pull.draft && <span className="xc-badge">{t("Draft")}</span>}
          {pull.state === "open" && (
            <>
              <span className={`xc-badge ${reviewTone(pull.reviewState)}`}>
                {t(reviewLabel[pull.reviewState])}
              </span>
              <span className={`xc-badge ${checkTone(pull.checkState)}`}>
                {t(checkLabel[pull.checkState])}
              </span>
            </>
          )}
          {pull.issueKeys.map((key) => (
            <Link key={key} className="xc-badge accent" to={issuePath(key)}>
              {key}
            </Link>
          ))}
          {pull.codingTaskId != null && (
            <Link className="xc-badge info" to={`/coding/${pull.codingTaskId}`}>
              <Bot size={12} aria-hidden /> {t("Coding task")} #
              {pull.codingTaskId}
            </Link>
          )}
        </div>
      </div>
    </li>
  );
}

/* ---- 提交 ---- */

function CommitsView({ sel }: { sel: Selection }) {
  const t = useT();
  const commits = useCommits(sel?.repo ?? "", sel?.connectionId ?? 0);
  if (commits.isError && isNotLive(commits.error))
    return (
      <NotLive name={t("Commits")} icon={<GitCommitHorizontal size={28} />} />
    );
  if (!commits.data || commits.data.length === 0)
    return <ListState query={commits} empty="No commits yet" />;
  // 全部仓库时按时间混排，每行前面带仓库名
  return (
    <section className="xc-card repos-card">
      <ul className="github-list">
        {commits.data.map((c) => (
          <CommitRow key={`${c.repo}@${c.sha}`} commit={c} showRepo={!sel} />
        ))}
      </ul>
    </section>
  );
}

function CommitRow({
  commit: c,
  showRepo,
}: {
  commit: GitHubCommit;
  showRepo: boolean;
}) {
  const language = useLanguage();
  const title = c.message.split("\n")[0];
  return (
    <li className="github-item repos-commit">
      <CheckIcon state={c.checkState} />
      <div className="github-item-main">
        <span className="github-title repos-commit-title" title={c.message}>
          {showRepo && (
            <span className="repos-commit-repo">
              <RepoSwatch repo={c.repo} />
              {splitRepo(c.repo).name}
            </span>
          )}
          {title}
        </span>
        <div className="github-meta">
          <a
            className="xc-mono repos-sha"
            href={c.url}
            target="_blank"
            rel="noreferrer"
          >
            {c.sha.slice(0, 7)}
          </a>
          <span>{c.author}</span>
          <span>{relativeTime(c.committedAt, language)}</span>
        </div>
      </div>
    </li>
  );
}

/* ---- CI ---- */

/** “第 3 / 8 步”。 */
function stepText(language: string, n: number, total: number) {
  return language === "zh" ? `第 ${n} / ${total} 步` : `step ${n} of ${total}`;
}

function RunsView({ sel }: { sel: Selection }) {
  const t = useT();
  const runs = useRuns();
  const items = (runs.data ?? []).filter((r) => !sel || sameRepo(r, sel));
  if (items.length === 0)
    return <ListState query={runs} empty="No workflow runs yet" />;
  const heads = latestDefaultRuns(items);
  return (
    <div className="xc-stack">
      {heads.length > 0 && (
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Default branches")}</h2>
          </div>
          <div className="github-heads">
            {heads.map((r) => {
              const o = runOutcome(r);
              return (
                <a
                  key={r.id}
                  className={`xc-badge ${o.tone}`}
                  href={r.url}
                  target="_blank"
                  rel="noreferrer"
                >
                  {!sel && <RepoSwatch repo={r.repo} />}
                  {!sel && `${splitRepo(r.repo).name} · `}
                  {r.name} · {t(o.label)}
                </a>
              );
            })}
          </div>
        </section>
      )}
      <section className="xc-card repos-card">
        <ul className="github-list repos-runs">
          {items.map((r) => (
            <RunItem key={r.id} run={r} showRepo={!sel} />
          ))}
        </ul>
      </section>
    </div>
  );
}

function RunIcon({ state }: { state: ReturnType<typeof stepState> }) {
  switch (state) {
    case "done":
      return <CircleCheck size={14} className="github-check ok" />;
    case "failed":
      return <CircleX size={14} className="github-check danger" />;
    case "running":
      return <Loader size={14} className="github-check warn github-spin" />;
    case "skipped":
      return <CircleMinus size={14} className="github-check" />;
    default:
      return <CircleDashed size={14} className="github-check" />;
  }
}

function RunItem({ run, showRepo }: { run: GitHubRun; showRepo: boolean }) {
  const t = useT();
  const language = useLanguage();
  const o = runOutcome(run);
  const running = run.status !== "completed";
  const [open, setOpen] = useState(false);
  return (
    <li className={`repos-run${open ? " open" : ""}`}>
      <button
        type="button"
        className="repos-run-head"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronRight size={13} className="repos-run-chevron" />
        <RunIcon state={stepState(run)} />
        <span className="repos-run-main">
          <span className="repos-run-title">
            {showRepo && (
              <span className="repos-commit-repo">
                <RepoSwatch repo={run.repo} />
                {splitRepo(run.repo).name}
              </span>
            )}
            {run.name}
          </span>
          <span className="github-meta">
            <span className="xc-mono github-branch">{run.branch}</span>
            {run.defaultBranch && (
              <span className="xc-badge github-inline">{t("default")}</span>
            )}
            <span>{run.event}</span>
            {run.headSha && (
              <span className="xc-mono">{run.headSha.slice(0, 7)}</span>
            )}
            <span>{relativeTime(run.createdAt, language)}</span>
          </span>
        </span>
        <span className={`xc-badge ${o.tone} repos-run-result`}>
          {t(o.label)}
          {running && run.stepsTotal
            ? ` · ${stepText(language, Math.min((run.stepsDone ?? 0) + 1, run.stepsTotal), run.stepsTotal)}`
            : ""}
        </span>
      </button>
      {running && run.currentStep && !open && (
        <p className="repos-run-current xc-muted">
          <Loader size={11} className="github-spin" /> {run.currentStep}
        </p>
      )}
      {open && <RunJobs run={run} />}
    </li>
  );
}

function RunJobs({ run }: { run: GitHubRun }) {
  const t = useT();
  const language = useLanguage();
  const jobs = useRunJobs(run, true);
  if (jobs.isPending) return <Loading />;
  if (jobs.isError)
    return (
      <div className="repos-jobs-note xc-muted">
        {isNotLive(jobs.error)
          ? t("Step details are not live yet.")
          : errorMessage(jobs.error)}{" "}
        <a href={run.url} target="_blank" rel="noreferrer">
          {t("Open in browser")}
        </a>
      </div>
    );
  if (jobs.data.length === 0)
    return (
      <div className="repos-jobs-note xc-muted">
        {t("This server does not report step details.")}
      </div>
    );
  const p = jobsProgress(jobs.data);
  return (
    <div className="repos-jobs">
      {run.status !== "completed" && p.total > 0 && (
        <div className="repos-progress">
          <div className="repos-progress-text">
            {p.current
              ? `${stepText(language, p.currentIndex ?? 0, p.total)}：${p.current}`
              : `${p.done} / ${p.total}`}
          </div>
          <div className="repos-bar">
            <span style={{ width: `${(p.done / p.total) * 100}%` }} />
          </div>
        </div>
      )}
      {jobs.data.map((job) => (
        <JobBlock key={job.id} job={job} />
      ))}
    </div>
  );
}

function JobBlock({ job }: { job: GitHubJob }) {
  const language = useLanguage();
  const state = stepState(job);
  // 正在跑的和失败的 job 默认展开
  const [open, setOpen] = useState(state === "running" || state === "failed");
  const done = job.steps.filter((s) => s.status === "completed").length;
  const current = job.steps.find((s) => s.status === "in_progress");
  return (
    <div className={`repos-job ${state}`}>
      <button
        type="button"
        className="repos-job-head"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronRight size={12} className="repos-run-chevron" />
        <RunIcon state={state} />
        <span className="repos-job-name">{job.name}</span>
        <small className="xc-muted">
          {state === "running" && current
            ? stepText(language, current.number, job.steps.length)
            : `${done} / ${job.steps.length}`}
        </small>
        <small className="xc-muted repos-job-time">
          {duration(job.startedAt, job.completedAt)}
        </small>
      </button>
      {open && (
        <ol className="repos-steps">
          {job.steps.map((s) => {
            const st = stepState(s);
            return (
              <li key={s.number} className={`repos-step ${st}`}>
                <RunIcon state={st} />
                <span className="repos-step-name">{s.name}</span>
                <small className="xc-muted">
                  {duration(s.startedAt, s.completedAt)}
                </small>
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}

/* ---- Issue ---- */

const relationLabel: Record<GitHubIssue["relation"], string> = {
  assigned: "Assigned to me",
  created: "Created by me",
  both: "Mine",
};

function IssuesView({ sel }: { sel: Selection }) {
  const t = useT();
  const language = useLanguage();
  const issues = useGitHubIssues();
  const items = (issues.data ?? []).filter((i) => !sel || sameRepo(i, sel));
  if (items.length === 0)
    return <ListState query={issues} empty="No open issues for you" />;
  return (
    <Grouped
      items={items}
      sel={sel}
      render={(is) => (
        <li className="github-item" key={`${is.repo}#${is.number}`}>
          <CircleDot size={15} className="github-check ok" aria-hidden />
          <div className="github-item-main">
            <a
              className="github-title"
              href={is.url}
              target="_blank"
              rel="noreferrer"
            >
              {is.title} <span className="xc-muted">#{is.number}</span>
            </a>
            <div className="github-meta">
              <span>{is.author}</span>
              <span>{relativeTime(is.updatedAt, language)}</span>
            </div>
            <div className="github-badges">
              <span className="xc-badge info">
                {t(relationLabel[is.relation])}
              </span>
              {is.labels.map((l) => (
                <span className="xc-badge" key={l}>
                  {l}
                </span>
              ))}
            </div>
          </div>
        </li>
      )}
    />
  );
}
