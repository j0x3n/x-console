import { useLocation, useSearchParams } from "react-router";
import NavChildLinks, {
  type NavChildLink,
} from "../../components/layout/NavChildLinks";
import { NAV_CHILD_LIMIT, type NavChildrenProps } from "../../lib/navChildren";
import { useBoards, useProjects, useStarredBoards } from "./api";

/** 标星的看板最多显示几条。 */
const STARRED_LIMIT = 8;

/**
 * 侧边栏“项目”下面：最近更新的项目。
 * 正在看的项目下面缩进列出它的全部看板，方便切换；其他项目只列标星的看板（B46）。
 */
export default function ProjectsNavChildren({ onNavigate }: NavChildrenProps) {
  const projects = useProjects();
  const starred = useStarredBoards();
  const { pathname } = useLocation();
  const [search] = useSearchParams();
  const list = [...(projects.data ?? [])]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, NAV_CHILD_LIMIT);
  const activeKey = /^\/projects\/([A-Za-z]{2,5})(?:\/|$)/
    .exec(pathname)?.[1]
    ?.toUpperCase();
  const currentBoard = search.get("board");
  const stars = (starred.data ?? []).slice(0, STARRED_LIMIT);
  const activeId = projects.data?.find((p) => p.key === activeKey)?.id;
  const activeBoards = (useBoards(activeId).data ?? []).filter(
    (b) => !b.archivedAt,
  );

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
      hint: p.key,
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
    <NavChildLinks
      links={links}
      total={projects.data?.length ?? 0}
      limit={links.length}
      allTo="/projects"
      loading={projects.isPending}
      error={projects.isError}
      empty="还没有项目"
      onNavigate={onNavigate}
    />
  );
}
