import NavChildLinks from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useTasks } from "./api";
import { FILTERS, filterTasks } from "./logic";

/** 侧边栏“Agent 任务”下面：按状态分组，右边是数量。 */
export default function CodingNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const tasks = useTasks();
  const groups = FILTERS.filter((f) => f.id !== "all");
  return (
    <NavChildLinks
      links={groups.map((f) => {
        const n = filterTasks(tasks.data ?? [], f.id).length;
        return {
          key: f.id,
          to: `/coding?filter=${f.id}`,
          label: t(f.label),
          hint: tasks.data ? String(n) : undefined,
          active: false,
        };
      })}
      allTo="/coding"
      error={tasks.isError}
      empty=""
      onNavigate={onNavigate}
    />
  );
}
