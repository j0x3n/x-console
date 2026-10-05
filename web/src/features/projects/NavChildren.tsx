import { useState } from "react";
import {
  NavLink,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router";
import {
  AlarmClock,
  Archive,
  CalendarRange,
  CircleAlert,
  CircleUser,
  LayoutGrid,
  Search,
} from "lucide-react";
import NavChildLinks, {
  type NavChildLink,
} from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useBoards, useMyIssues, useProjects, useStarredBoards } from "./api";
import { issuesInView, localDate, VIEW_LABELS, type IssueView } from "./logic";

/** 标星的看板最多显示几条。 */
const STARRED_LIMIT = 8;

const viewIcons: Record<IssueView, typeof CircleUser> = {
  mine: CircleUser,
  today: AlarmClock,
  overdue: CircleAlert,
  week: CalendarRange,
};

/**
 * 左栏“项目”的二级菜单（B101）：
 * 搜索框、全部项目、四个视图，下面是项目列表，最后是已归档。
 * 正在看的项目下面缩进列出它的全部看板；其他项目只列标星的看板（B46）。
 */
export default function ProjectsNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const navigate = useNavigate();
  const projects = useProjects();
  const archived = useProjects(true);
  const myIssues = useMyIssues();
  const starred = useStarredBoards();
  const { pathname } = useLocation();
  const [search] = useSearchParams();
  const [query, setQuery] = useState("");
  const list = [...(projects.data ?? [])].sort((a, b) =>
    b.updatedAt.localeCompare(a.updatedAt),
  );
  const activeKey = /^\/projects\/([A-Z]{2,5})(?:\/|$)/.exec(pathname)?.[1];
  const currentBoard = search.get("board");
  const stars = (starred.data ?? []).slice(0, STARRED_LIMIT);
  const activeId = projects.data?.find((p) => p.key === activeKey)?.id;
  const activeBoards = (useBoards(activeId).data ?? []).filter(
    (b) => !b.archivedAt,
  );
  const today = localDate();
  const count = (view: IssueView) =>
    myIssues.data ? issuesInView(view, myIssues.data, today).length : null;
  const atList = pathname === "/projects";
  const showingArchived = atList && search.get("archived") === "1";

  const links: NavChildLink[] = [];
  for (const p of list) {
    links.push({
      key: p.id,
      to: `/projects/${p.key}`,
      label: p.name,
      mark: (
        <i
          className="nav-child-swatch"
          style={{ background: p.color || "var(--xc-accent)" }}
        />
      ),
      hint: String(p.openCount),
      active: p.key === activeKey && currentBoard === null,
    });
    const nested =
      p.id === activeId && activeBoards.length > 1
        ? activeBoards
        : stars.filter((s) => s.projectId === p.id);
    for (const b of nested)
      links.push({
        key: `b${b.id}`,
        to: `/projects/${p.key}?board=${b.id}`,
        label: `${b.icon ? b.icon + " " : ""}${b.name}`,
        nested: true,
        active: p.key === activeKey && currentBoard === String(b.id),
      });
  }

  return (
    <div className="projects-nav">
      <form
        className="projects-nav-search"
        role="search"
        onSubmit={(e) => {
          e.preventDefault();
          const q = query.trim();
          navigate(
            `/projects/views/mine${q ? `?q=${encodeURIComponent(q)}` : ""}`,
          );
          onNavigate();
        }}
      >
        <Search size={14} />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t("Search issues")}
          aria-label={t("Search issues")}
        />
      </form>
      <div className="nav-children">
        <NavLink
          to="/projects"
          end
          onClick={onNavigate}
          className={`nav-child${atList && !showingArchived ? " selected" : ""}`}
        >
          <LayoutGrid size={16} className="nav-child-mark" />
          <span>{t("Every project")}</span>
          {projects.data && <small>{projects.data.length}</small>}
        </NavLink>
        {(Object.keys(VIEW_LABELS) as IssueView[]).map((view) => {
          const Icon = viewIcons[view];
          const n = count(view);
          return (
            <NavLink
              key={view}
              to={`/projects/views/${view}`}
              onClick={onNavigate}
              className={({ isActive }) =>
                `nav-child${isActive ? " selected" : ""}`
              }
            >
              <Icon size={16} className="nav-child-mark" />
              <span>{t(VIEW_LABELS[view])}</span>
              {n !== null && (
                <small className={view === "overdue" && n > 0 ? "danger" : ""}>
                  {n}
                </small>
              )}
            </NavLink>
          );
        })}
      </div>
      <div className="projects-nav-group">
        <div className="projects-nav-label">{t("My projects")}</div>
        <NavChildLinks
          links={links}
          total={list.length}
          limit={links.length}
          allTo="/projects"
          loading={projects.isPending}
          error={projects.isError}
          empty={t("No projects yet")}
          onNavigate={onNavigate}
        />
      </div>
      {!!archived.data?.length && (
        <div className="projects-nav-group nav-children">
          <NavLink
            to="/projects?archived=1"
            onClick={onNavigate}
            className={`nav-child${showingArchived ? " selected" : ""}`}
          >
            <Archive size={16} className="nav-child-mark" />
            <span>{t("Archived")}</span>
            <small>{archived.data.length}</small>
          </NavLink>
        </div>
      )}
    </div>
  );
}
