import { Link, useSearchParams } from "react-router";
import {
  AlertTriangle,
  Bot,
  CircleCheck,
  CircleDashed,
  CircleDot,
  CircleX,
  ExternalLink,
  GitPullRequest,
  Github,
  RefreshCw,
  Settings,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  isNotConfigured,
  useGitHubIssues,
  useGitHubStatus,
  usePulls,
  useRuns,
  useSyncGitHub,
  type CheckState,
  type GitHubIssue,
  type GitHubPull,
  type GitHubRun,
  type GitHubStatus,
} from "./api";
import {
  checkLabel,
  checkTone,
  groupByRepo,
  issuePath,
  latestDefaultRuns,
  reviewLabel,
  reviewTone,
  runOutcome,
  sortPulls,
} from "./logic";

type Tab = "pulls" | "runs" | "issues";
const tabs: Tab[] = ["pulls", "runs", "issues"];

export default function GitHubPage() {
  const t = useT();
  const status = useGitHubStatus();
  const [params, setParams] = useSearchParams();
  const tab: Tab = tabs.includes(params.get("tab") as Tab)
    ? (params.get("tab") as Tab)
    : "pulls";
  const setTab = (next: Tab) =>
    setParams(next === "pulls" ? {} : { tab: next }, { replace: true });

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
            <span>还没有关注的仓库。在设置里加上 owner/name 形式的仓库。</span>
            <Link className="xc-btn small" to="/settings/github">
              {t("Settings")}
            </Link>
          </div>
        )}
        <GitHubStats status={status.data} />
        <nav className="xc-tabs">
          <button
            className={tab === "pulls" ? "active" : ""}
            onClick={() => setTab("pulls")}
          >
            {t("Pull requests")}
          </button>
          <button
            className={tab === "runs" ? "active" : ""}
            onClick={() => setTab("runs")}
          >
            {t("CI runs")}
          </button>
          <button
            className={tab === "issues" ? "active" : ""}
            onClick={() => setTab("issues")}
          >
            {t("Issues")}
          </button>
        </nav>
        {tab === "pulls" && <PullsView />}
        {tab === "runs" && <RunsView />}
        {tab === "issues" && <IssuesView />}
      </>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("GitHub")}
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
    <StatStrip label={t("GitHub")}>
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
      <button className="xc-btn small" onClick={onSync} disabled={busy}>
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
    <EmptyState title={t("Connect GitHub")} icon={<Github size={28} />}>
      <span>
        填一个 GitHub 令牌和要关注的仓库，就能在这里看 PR、CI 和 Issue。
      </span>
      <Link className="xc-btn small primary" to="/settings/github">
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

function PullsView() {
  const pulls = usePulls();
  if (!pulls.data || pulls.data.length === 0)
    return <ListState query={pulls} empty="No recent pull requests" />;
  return (
    <div className="xc-stack">
      {groupByRepo(sortPulls(pulls.data)).map((g) => (
        <section className="xc-card" key={g.repo}>
          <div className="xc-card-head">
            <h2 className="xc-mono">{g.repo}</h2>
            <span className="xc-badge">{g.items.length}</span>
          </div>
          <ul className="github-list">
            {g.items.map((p) => (
              <PullRow key={p.number} pull={p} />
            ))}
          </ul>
        </section>
      ))}
    </div>
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

function RunsView() {
  const t = useT();
  const runs = useRuns();
  if (!runs.data || runs.data.length === 0)
    return <ListState query={runs} empty="No workflow runs yet" />;
  const heads = latestDefaultRuns(runs.data);
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
                  {r.repo} · {r.name} · {t(o.label)}
                </a>
              );
            })}
          </div>
        </section>
      )}
      <section className="xc-card">
        <div className="xc-table-wrap">
          <table className="xc-table">
            <thead>
              <tr>
                <th>{t("Workflow")}</th>
                <th>{t("Repository")}</th>
                <th>{t("Branch")}</th>
                <th>{t("Result")}</th>
                <th>{t("Started at")}</th>
              </tr>
            </thead>
            <tbody>
              {runs.data.map((r) => (
                <RunRow key={r.id} run={r} />
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function RunRow({ run }: { run: GitHubRun }) {
  const t = useT();
  const language = useLanguage();
  const o = runOutcome(run);
  return (
    <tr>
      <td>
        <a
          href={run.url}
          target="_blank"
          rel="noreferrer"
          className="github-title"
        >
          {run.name} <ExternalLink size={12} aria-hidden />
        </a>
        <div className="xc-muted github-small">{run.event}</div>
      </td>
      <td className="xc-mono">{run.repo}</td>
      <td className="xc-mono">
        {run.branch}
        {run.defaultBranch && (
          <span className="xc-badge github-inline">{t("default")}</span>
        )}
      </td>
      <td>
        <span className={`xc-badge ${o.tone}`}>{t(o.label)}</span>
      </td>
      <td className="xc-muted">{relativeTime(run.createdAt, language)}</td>
    </tr>
  );
}

const relationLabel: Record<GitHubIssue["relation"], string> = {
  assigned: "Assigned to me",
  created: "Created by me",
  both: "Mine",
};

function IssuesView() {
  const t = useT();
  const language = useLanguage();
  const issues = useGitHubIssues();
  if (!issues.data || issues.data.length === 0)
    return <ListState query={issues} empty="No open issues for you" />;
  return (
    <div className="xc-stack">
      {groupByRepo(issues.data).map((g) => (
        <section className="xc-card" key={g.repo}>
          <div className="xc-card-head">
            <h2 className="xc-mono">{g.repo}</h2>
            <span className="xc-badge">{g.items.length}</span>
          </div>
          <ul className="github-list">
            {g.items.map((is) => (
              <li className="github-item" key={is.number}>
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
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
