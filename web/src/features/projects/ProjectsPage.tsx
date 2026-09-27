import { useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { FolderKanban, Plus } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useMyIssues, useProjects, type Project } from "./api";
import { useIssueCommands, useProjectCommands } from "./commands";
import { ProjectBadge } from "./components/Icons";
import IssueList from "./components/IssueList";
import NewIssueDialog from "./components/NewIssueDialog";
import ProjectDialog from "./components/ProjectDialog";
import { issuePath, parseIssueKey, type IssueGroup } from "./logic";

export default function ProjectsPage() {
  const t = useT();
  const navigate = useNavigate();
  const [search, setSearch] = useSearchParams();
  const [showArchived, setShowArchived] = useState(false);
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
  return (
    <div className="xc-page projects-page">
      <PageHeading
        title={t("Projects")}
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
        <section className="projects-mine">
          <h2>{t("My open issues")}</h2>
          <p className="xc-muted">
            {t("Open issues in all projects. The ones due soonest come first.")}
          </p>
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
        </section>
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

function ProjectCard({ project }: { project: Project }) {
  const t = useT();
  const done = project.issueCount - project.openCount;
  const pct = project.issueCount
    ? Math.round((done / project.issueCount) * 100)
    : 0;
  return (
    <Link
      to={`/projects/${project.key}`}
      className="xc-card projects-card-link"
    >
      <div className="xc-row">
        <ProjectBadge projectKey={project.key} color={project.color} />
        <strong>{project.name}</strong>
        <span className="xc-spacer" />
        <span className="xc-mono xc-muted">{project.key}</span>
      </div>
      {project.description && (
        <p className="projects-card-desc">{project.description}</p>
      )}
      <div className="projects-progress" aria-label={`${pct}%`}>
        <i
          style={{ width: `${pct}%`, background: project.color || undefined }}
        />
      </div>
      <small className="xc-muted">
        {project.openCount} {t("open")} · {project.issueCount} {t("total")}
      </small>
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
