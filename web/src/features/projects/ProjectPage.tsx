import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { Columns3, LayoutList, Pencil, Plus, Settings2 } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import {
  useIssues,
  useLabels,
  useMilestones,
  useMoveIssue,
  useProjectByKey,
  useUpdateIssue,
} from "./api";
import { useIssueCommands } from "./commands";
import Board from "./components/Board";
import FilterBar from "./components/FilterBar";
import { ProjectBadge } from "./components/Icons";
import IssueList from "./components/IssueList";
import NewIssueDialog, { rememberProject } from "./components/NewIssueDialog";
import ProjectDialog from "./components/ProjectDialog";
import ProjectSettingsDialog from "./components/ProjectSettingsDialog";
import {
  STATUSES,
  emptyFilter,
  filterIssues,
  groupIssues,
  issuePath,
  stepSelection,
  type GroupBy,
  type IssueFilter,
  type IssueStatus,
  type SortKey,
} from "./logic";
import { useShortcuts } from "./useShortcuts";

type View = "board" | "list";

interface ViewPrefs {
  view: View;
  groupBy: GroupBy;
  sort: SortKey;
}

const PREFS_KEY = "projects.view";

function loadPrefs(): ViewPrefs {
  const fallback: ViewPrefs = {
    view: "board",
    groupBy: "status",
    sort: "manual",
  };
  try {
    return {
      ...fallback,
      ...JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}"),
    };
  } catch {
    return fallback;
  }
}

function savePrefs(prefs: ViewPrefs) {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch {
    /* 忽略 */
  }
}

export default function ProjectPage() {
  const t = useT();
  const navigate = useNavigate();
  const { projectKey } = useParams();
  const { project, isPending, error } = useProjectByKey(projectKey);
  const issues = useIssues(project?.id);
  const labels = useLabels(project?.id);
  const milestones = useMilestones(project?.id);
  const move = useMoveIssue();
  const update = useUpdateIssue();
  const [prefs, setPrefs] = useState(loadPrefs);
  const [filter, setFilter] = useState<IssueFilter>(emptyFilter);
  const [selected, setSelected] = useState<string | null>(null);
  const [newIssue, setNewIssue] = useState<IssueStatus | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  useIssueCommands(issues.data);

  useEffect(() => {
    if (project) rememberProject(project.id);
  }, [project]);

  const changePrefs = (next: Partial<ViewPrefs>) => {
    const merged = { ...prefs, ...next };
    setPrefs(merged);
    savePrefs(merged);
  };

  const visible = useMemo(
    () => filterIssues(issues.data ?? [], filter),
    [issues.data, filter],
  );
  const groups = useMemo(
    () =>
      prefs.view === "board"
        ? groupIssues(visible, "status", "manual")
        : groupIssues(visible, prefs.groupBy, prefs.sort),
    [visible, prefs],
  );
  // J/K 按屏幕上的顺序移动。
  const order = useMemo(
    () => groups.flatMap((g) => g.issues.map((i) => i.key)),
    [groups],
  );

  const selectedIssue = issues.data?.find((i) => i.key === selected);
  const open = (key: string) => navigate(issuePath(key));

  useEffect(() => {
    if (!selected) return;
    document
      .querySelector(`[data-issue-key="${selected}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [selected]);

  useShortcuts(
    {
      c: () => setNewIssue("todo"),
      j: () => setSelected((k) => stepSelection(order, k, 1)),
      k: () => setSelected((k) => stepSelection(order, k, -1)),
      enter: (event) =>
        // 焦点在卡片或行上时，由它自己处理 Enter。
        selected && !(event.target as HTMLElement).closest?.("[data-issue-key]")
          ? open(selected)
          : false,
      e: () =>
        selected ? navigate(`${issuePath(selected)}?edit=title`) : false,
      escape: () => setSelected(null),
      ...Object.fromEntries(
        STATUSES.map((status, i) => [
          String(i + 1),
          () => {
            if (!selectedIssue) return false;
            if (selectedIssue.status !== status)
              update.mutate({
                issue: selectedIssue,
                key: selectedIssue.key,
                body: { status },
              });
          },
        ]),
      ),
    },
    !!project,
  );

  if (isPending) return <Loading />;
  if (error) return <ErrorState error={error} />;
  if (!project)
    return (
      <div className="xc-page">
        <EmptyState title={t("Project not found")}>
          <Link to="/projects">{t("Back to projects")}</Link>
        </EmptyState>
      </div>
    );

  return (
    <div className="xc-page wide projects-page">
      <PageHeading
        title={project.name}
        subtitle={project.description || undefined}
        aside={
          <>
            <button
              className="xc-btn small"
              onClick={() => setEditOpen(true)}
              title={t("Edit")}
            >
              <Pencil size={14} /> {t("Edit")}
            </button>
            <button
              className="xc-btn small"
              onClick={() => setSettingsOpen(true)}
              aria-label={t("Settings")}
            >
              <Settings2 size={14} />
            </button>
            <button
              className="xc-btn primary small"
              onClick={() => setNewIssue("todo")}
              disabled={!!project.archivedAt}
            >
              <Plus size={14} /> {t("New issue")}
              <kbd className="projects-kbd">C</kbd>
            </button>
          </>
        }
      />
      <div className="projects-toolbar">
        <ProjectBadge projectKey={project.key} color={project.color} />
        <div className="projects-view-switch" role="tablist">
          <button
            role="tab"
            aria-selected={prefs.view === "board"}
            className={prefs.view === "board" ? "active" : ""}
            onClick={() => changePrefs({ view: "board" })}
          >
            <Columns3 size={14} /> {t("Board")}
          </button>
          <button
            role="tab"
            aria-selected={prefs.view === "list"}
            className={prefs.view === "list" ? "active" : ""}
            onClick={() => changePrefs({ view: "list" })}
          >
            <LayoutList size={14} /> {t("List")}
          </button>
        </div>
        {prefs.view === "list" && (
          <>
            <select
              className="xc-select projects-filter-select"
              value={prefs.groupBy}
              aria-label={t("Group by")}
              onChange={(e) =>
                changePrefs({ groupBy: e.target.value as GroupBy })
              }
            >
              <option value="status">{t("Group by status")}</option>
              <option value="priority">{t("Group by priority")}</option>
              <option value="none">{t("No grouping")}</option>
            </select>
            <select
              className="xc-select projects-filter-select"
              value={prefs.sort}
              aria-label={t("Sort")}
              onChange={(e) => changePrefs({ sort: e.target.value as SortKey })}
            >
              <option value="manual">{t("Manual order")}</option>
              <option value="updated">{t("Recently updated")}</option>
              <option value="priority">{t("Priority")}</option>
              <option value="due">{t("Due date")}</option>
            </select>
          </>
        )}
        <span className="xc-spacer" />
        <span className="xc-muted projects-hint">
          {t("C new · J/K move · Enter open · 1-6 status")}
        </span>
      </div>
      <FilterBar
        filter={filter}
        onChange={setFilter}
        labels={labels.data ?? []}
        milestones={milestones.data ?? []}
        showStatus={prefs.view === "list"}
      />
      {issues.isPending ? (
        <Loading />
      ) : issues.isError ? (
        <ErrorState error={issues.error} onRetry={() => issues.refetch()} />
      ) : prefs.view === "board" ? (
        <Board
          issues={visible}
          selectedKey={selected}
          onSelect={setSelected}
          onOpen={open}
          onAdd={(status) => setNewIssue(status)}
          onMove={(key, plan) =>
            move.mutate({ projectId: project.id, key, plan })
          }
        />
      ) : (
        <IssueList
          groups={groups}
          selectedKey={selected}
          onSelect={setSelected}
          onOpen={open}
        />
      )}
      <NewIssueDialog
        open={newIssue !== null}
        onClose={() => setNewIssue(null)}
        projectId={project.id}
        status={newIssue ?? "todo"}
        onCreated={(issue) => setSelected(issue.key)}
      />
      <ProjectDialog
        open={editOpen}
        onClose={() => setEditOpen(false)}
        project={project}
      />
      <ProjectSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        project={project}
      />
    </div>
  );
}
