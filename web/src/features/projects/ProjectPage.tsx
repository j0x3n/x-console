import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import {
  Columns3,
  Keyboard,
  LayoutList,
  Pencil,
  Plus,
  Settings2,
} from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { Segmented, Toolbar } from "../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import {
  useB36Live,
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
import IssueList from "./components/IssueList";
import NewIssueDialog, { rememberProject } from "./components/NewIssueDialog";
import ProjectDialog from "./components/ProjectDialog";
import ProjectSettingsDialog from "./components/ProjectSettingsDialog";
import ShortcutsDialog from "./components/ShortcutsDialog";
import {
  STATUSES,
  UNCATEGORIZED,
  categoryPaths,
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
  const { categories } = useB36Live(project?.id);
  const move = useMoveIssue();
  const update = useUpdateIssue();
  const [prefs, setPrefs] = useState(loadPrefs);
  const [baseFilter, setBaseFilter] = useState<IssueFilter>(emptyFilter);
  const [search, setSearch] = useSearchParams();
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [newIssue, setNewIssue] = useState<IssueStatus | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  useIssueCommands(issues.data);

  useEffect(() => {
    if (project) rememberProject(project.id);
  }, [project]);

  // 分类筛选放在地址里（?category=3），侧边栏的分类链接直接打开。
  const categoryParam = search.get("category");
  const filter: IssueFilter = useMemo(
    () => ({
      ...baseFilter,
      categoryId:
        categoryParam === null || Number.isNaN(Number(categoryParam))
          ? null
          : Number(categoryParam),
    }),
    [baseFilter, categoryParam],
  );
  const setFilter = (next: IssueFilter) => {
    setBaseFilter({ ...next, categoryId: null });
    if (next.categoryId !== filter.categoryId) {
      const params = new URLSearchParams(search);
      if (next.categoryId === null) params.delete("category");
      else params.set("category", String(next.categoryId));
      setSearch(params, { replace: true });
    }
  };
  const categoryNames = useMemo(() => categoryPaths(categories), [categories]);
  // 没有分类时，“按分类分组”退回按状态分组。
  const groupBy: GroupBy =
    prefs.groupBy === "category" && categories.length === 0
      ? "status"
      : prefs.groupBy;

  const changePrefs = (next: Partial<ViewPrefs>) => {
    const merged = { ...prefs, ...next };
    setPrefs(merged);
    savePrefs(merged);
  };

  const visible = useMemo(
    () => filterIssues(issues.data ?? [], filter, categories),
    [issues.data, filter, categories],
  );
  const groups = useMemo(
    () =>
      prefs.view === "board"
        ? groupIssues(visible, "status", "manual")
        : groupIssues(visible, groupBy, prefs.sort, categories),
    [visible, prefs.view, prefs.sort, groupBy, categories],
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
      "?": () => setShortcutsOpen(true),
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
              onClick={() => setShortcutsOpen(true)}
              aria-label={t("Keyboard shortcuts")}
              title={`${t("Keyboard shortcuts")} (?)`}
            >
              <Keyboard size={14} />
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
      <Toolbar
        className="projects-toolbar"
        start={
          <FilterBar
            filter={filter}
            onChange={setFilter}
            labels={labels.data ?? []}
            milestones={milestones.data ?? []}
            categories={categories}
            showStatus={prefs.view === "list"}
            extra={
              prefs.view === "list" && (
                <>
                  <select
                    className="xc-select projects-filter-select"
                    value={groupBy}
                    aria-label={t("Group by")}
                    onChange={(e) =>
                      changePrefs({ groupBy: e.target.value as GroupBy })
                    }
                  >
                    <option value="status">{t("Group by status")}</option>
                    <option value="priority">{t("Group by priority")}</option>
                    {categories.length > 0 && (
                      <option value="category">{t("Group by category")}</option>
                    )}
                    <option value="none">{t("No grouping")}</option>
                  </select>
                  <select
                    className="xc-select projects-filter-select"
                    value={prefs.sort}
                    aria-label={t("Sort")}
                    onChange={(e) =>
                      changePrefs({ sort: e.target.value as SortKey })
                    }
                  >
                    <option value="manual">{t("Manual order")}</option>
                    <option value="updated">{t("Recently updated")}</option>
                    <option value="priority">{t("Priority")}</option>
                    <option value="due">{t("Due date")}</option>
                  </select>
                </>
              )
            }
          />
        }
        end={
          <Segmented
            label={t("View")}
            value={prefs.view}
            onChange={(view) => changePrefs({ view })}
            options={[
              {
                value: "board",
                label: t("Board"),
                icon: Columns3,
                iconOnly: true,
              },
              {
                value: "list",
                label: t("List"),
                icon: LayoutList,
                iconOnly: true,
              },
            ]}
          />
        }
      />
      {issues.isPending ? (
        <Loading />
      ) : issues.isError ? (
        <ErrorState error={issues.error} onRetry={() => issues.refetch()} />
      ) : prefs.view === "board" ? (
        <Board
          issues={visible}
          categoryNames={categoryNames}
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
          categoryNames={categoryNames}
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
        categoryId={
          filter.categoryId === UNCATEGORIZED ? null : filter.categoryId
        }
        onCreated={(issue) => setSelected(issue.key)}
      />
      <ProjectDialog
        open={editOpen}
        onClose={() => setEditOpen(false)}
        project={project}
      />
      <ShortcutsDialog
        open={shortcutsOpen}
        onClose={() => setShortcutsOpen(false)}
      />
      <ProjectSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        project={project}
      />
    </div>
  );
}
