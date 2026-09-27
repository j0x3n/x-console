import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useProjects } from "./api";

/** 侧边栏“项目”下面：最近更新的项目。 */
export default function ProjectsNavChildren({ onNavigate }: NavChildrenProps) {
  const projects = useProjects();
  const list = [...(projects.data ?? [])].sort((a, b) =>
    b.updatedAt.localeCompare(a.updatedAt),
  );
  return (
    <NavChildLinks
      links={list.map((p) => ({
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
      }))}
      allTo="/projects"
      loading={projects.isPending}
      error={projects.isError}
      empty="还没有项目"
      onNavigate={onNavigate}
    />
  );
}
