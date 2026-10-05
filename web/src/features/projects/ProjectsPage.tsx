import { useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { FolderKanban, Plus } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import {
  Section,
  Segments,
  StatCard,
  StatStrip,
} from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useMyIssues, useProjects, type Project } from "./api";
import { useIssueCommands, useProjectCommands } from "./commands";
import { ProjectBadge } from "./components/Icons";
import IssueList from "./components/IssueList";
import NewIssueDialog from "./components/NewIssueDialog";
import ProjectDialog from "./components/ProjectDialog";
import {
  dueState,
  issuePath,
  localDate,
  parseIssueKey,
  type Issue,
  type IssueGroup,
} from "./logic";

export default function ProjectsPage() {
  const t = useT();
  const navigate = useNavigate();
  const [search, setSearch] = useSearchParams();
  // 左栏“已归档”链到 /projects?archived=1（B101）
  const showArchived = search.get("archived") === "1";
  const setShowArchived = (update: (v: boolean) => boolean) => {
    if (update(showArchived)) search.set("archived", "1");
    else search.delete("archived");
    setSearch(search, { replace: true });
  };
  const projects = useProjects(false);
  const archived = useProjects(true);
  const myIssues = useMyIssues();
  useProjectCommands(projects.data);
  useIssueCommands(myIssues.data);

  const flag = (name: string) => search.get(name) === "1";
  const closeFlag = (name: string) => {
    search.delete(name);
    setSearch(search, { replace: true });
  };
  const [projectDialog, setProjectDialog] = useState(false);
  const newProjectOpen = projectDialog || flag("newProject");

  const groups = useMemo<IssueGroup[]>(
    () => [{ id: "mine", label: "Open issues", issues: myIssues.data ?? [] }],
    [myIssues.data],
  );

  const list = showArchived ? archived : projects;
  const open = myIssues.data ?? [];
  const summary = summarize(open);
  const projectCount = projects.data?.length ?? 0;
  return (
    <div className="xc-page projects-page">
      <PageHeading
        title={t("Projects")}
        subtitle={
          projectCount > 0 ? (
            <>
              {projectCount} {t("projects")} · <strong>{open.length}</strong>{" "}
              {t("open issues")}
              {summary.overdue > 0 && (
                <>
                  {" "}
                  · {summary.overdue} {t("overdue")}
                </>
              )}
            </>
          ) : undefined
        }
        aside={
          <>
            <button
              className="xc-btn small"
              onClick={() => setProjectDialog(true)}
            >
              <FolderKanban size={14} /> {t("New project")}
            </button>
            <button
              className="xc-btn primary small"
              disabled={!projects.data?.length}
              onClick={() => {
                search.set("new", "1");
                setSearch(search);
              }}
            >
              <Plus size={14} /> {t("New issue")}
            </button>
          </>
        }
      />
      {projectCount > 0 && (
        <StatStrip label={t("Projects")}>
          <StatCard
            label={t("Open issues")}
            caption={t("Across projects")}
            value={open.length}
            foot={`${summary.inReview} ${t("in review")}`}
          >
            <Segments
              parts={[
                { value: summary.backlog, tone: "muted", label: t("Backlog") },
                { value: summary.todo, tone: "info", label: t("Todo") },
                {
                  value: summary.inProgress,
                  tone: "warn",
                  label: t("In progress"),
                },
                { value: summary.inReview, tone: "ok", label: t("In review") },
              ]}
            />
          </StatCard>
          <StatCard
            label={t("In progress")}
            value={summary.inProgress}
            foot={t("Issues in progress")}
          />
          <StatCard
            label={t("Due today")}
            value={summary.today}
            tone={summary.today ? "accent" : undefined}
            foot={t("Issues due today")}
          />
          <StatCard
            label={t("Due this week")}
            caption={t("Next 7 days")}
            value={summary.week}
            foot={t("Including today")}
          />
          <StatCard
            label={t("Overdue")}
            value={summary.overdue}
            tone={summary.overdue ? "danger" : "ok"}
            foot={
              summary.overdue ? t("Past the due date") : t("Nothing overdue")
            }
          />
        </StatStrip>
      )}
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 && !showArchived ? (
        <EmptyState
          title={t("No projects yet")}
          icon={<FolderKanban size={28} />}
        >
          <button
            className="xc-btn primary small"
            onClick={() => setProjectDialog(true)}
          >
            <Plus size={14} /> {t("Create your first project")}
          </button>
        </EmptyState>
      ) : (
        <div className="projects-grid">
          {list.data.map((p) => (
            <ProjectCard key={p.id} project={p} />
          ))}
        </div>
      )}
      {!!archived.data?.length && (
        <button
          className="xc-btn ghost small projects-archived-toggle"
          onClick={() => setShowArchived((v) => !v)}
        >
          {showArchived
            ? t("Show active projects")
            : `${t("Show archived projects")} (${archived.data.length})`}
        </button>
      )}

      {!!projects.data?.length && (
        <Section
          className="projects-mine"
          title={t("My open issues")}
          count={open.length}
          aside={
            <>
              <span className="xc-muted projects-hint">
                {t("Due soonest first")}
              </span>
              <Link className="projects-view-all" to="/projects/views/mine">
                {t("View all")}
              </Link>
            </>
          }
        >
          {myIssues.isPending ? (
            <Loading />
          ) : myIssues.isError ? (
            <ErrorState
              error={myIssues.error}
              onRetry={() => myIssues.refetch()}
            />
          ) : (
            <IssueList
              groups={groups}
              selectedKey={null}
              onSelect={() => {}}
              onOpen={(key) => navigate(issuePath(key))}
              showProject
            />
          )}
        </Section>
      )}

      <ProjectDialog
        open={newProjectOpen}
        onClose={() => {
          setProjectDialog(false);
          if (flag("newProject")) closeFlag("newProject");
        }}
        onSaved={(p) => navigate(`/projects/${p.key}`)}
      />
      <NewIssueDialog
        open={flag("new") && !!projects.data?.length}
        onClose={() => closeFlag("new")}
        onCreated={(issue) => navigate(issuePath(issue.key))}
      />
      <GotoIssueDialog open={flag("goto")} onClose={() => closeFlag("goto")} />
    </div>
  );
}

function summarize(issues: Issue[]) {
  const today = localDate();
  const weekEnd = localDate(new Date(Date.now() + 6 * 86_400_000));
  const s = {
    backlog: 0,
    todo: 0,
    inProgress: 0,
    inReview: 0,
    today: 0,
    week: 0,
    overdue: 0,
  };
  for (const issue of issues) {
    if (issue.status === "backlog") s.backlog++;
    else if (issue.status === "todo") s.todo++;
    else if (issue.status === "in_progress") s.inProgress++;
    else if (issue.status === "in_review") s.inReview++;
    const due = dueState(issue.dueDate, today, issue.status);
    if (due === "overdue") s.overdue++;
    if (due === "today") s.today++;
    if (issue.dueDate && issue.dueDate >= today && issue.dueDate <= weekEnd)
      s.week++;
  }
  return s;
}

/** 项目卡片（B101）：标识、名称、更新时间、两行描述、进度条和数量。 */
function ProjectCard({ project }: { project: Project }) {
  const t = useT();
  const language = useLanguage();
  const done = project.issueCount - project.openCount;
  const pct = project.issueCount
    ? Math.round((done / project.issueCount) * 100)
    : 0;
  return (
    <Link
      to={`/projects/${project.key}`}
      className="xc-card projects-card-link"
    >
      <div className="projects-card-top">
        <ProjectBadge projectKey={project.key} color={project.color} />
        <div className="projects-card-name">
          <strong>{project.name}</strong>
          <span className="xc-muted">
            {project.key} · {relativeTime(project.updatedAt, language)}
          </span>
        </div>
      </div>
      <p className="projects-card-desc">
        {project.description || t("No description")}
      </p>
      <div className="projects-card-progress">
        <div className="projects-card-foot">
          <span>
            <strong>{project.openCount}</strong> {t("open")} ·{" "}
            <strong>{done}</strong> {t("done")}
          </span>
          <span className="xc-spacer" />
          <span>{pct}%</span>
        </div>
        <span className="projects-card-bar" aria-hidden>
          <i
            style={{
              width: `${pct}%`,
              background: project.color || "var(--xc-accent)",
            }}
          />
        </span>
      </div>
    </Link>
  );
}

/** 输入 key（比如 XC-12）跳到 Issue。 */
function GotoIssueDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const navigate = useNavigate();
  const [value, setValue] = useState("");
  const parsed = parseIssueKey(value);
  return (
    <Dialog open={open} onClose={onClose} title={t("Go to issue")}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!parsed) return;
          onClose();
          setValue("");
          navigate(issuePath(value));
        }}
      >
        <label className="xc-field">
          <span>{t("Issue key")}</span>
          <input
            className="xc-input xc-mono"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="XC-12"
            autoFocus
          />
        </label>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={!parsed}>
            {t("Open")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
