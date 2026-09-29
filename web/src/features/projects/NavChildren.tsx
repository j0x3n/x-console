import { useLocation, useSearchParams } from "react-router";
import NavChildLinks, {
  type NavChildLink,
} from "../../components/layout/NavChildLinks";
import { NAV_CHILD_LIMIT, type NavChildrenProps } from "../../lib/navChildren";
import { useB36Live, useProjects } from "./api";
import { categoryTree } from "./logic";

/** 分类最多显示几条。 */
const CATEGORY_LIMIT = 5;

/**
 * 侧边栏“项目”下面：最近更新的项目。
 * 正在看某个项目时，它下面缩进列出一级分类（B36）。
 */
export default function ProjectsNavChildren({ onNavigate }: NavChildrenProps) {
  const projects = useProjects();
  const { pathname } = useLocation();
  const [search] = useSearchParams();
  const list = [...(projects.data ?? [])]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, NAV_CHILD_LIMIT);
  const activeKey = /^\/projects\/([A-Za-z]{2,5})(?:\/|$)/
    .exec(pathname)?.[1]
    ?.toUpperCase();
  const active = list.find((p) => p.key === activeKey);
  const { categories } = useB36Live(active?.id);
  const top = categoryTree(categories)
    .filter((n) => n.depth === 0)
    .slice(0, CATEGORY_LIMIT);
  const current = search.get("category");

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
      active: p.key === activeKey && current === null,
    });
    if (p.id === active?.id)
      for (const n of top)
        links.push({
          key: `c${n.category.id}`,
          to: `/projects/${p.key}?category=${n.category.id}`,
          label: n.category.name,
          nested: true,
          active: current === String(n.category.id),
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
