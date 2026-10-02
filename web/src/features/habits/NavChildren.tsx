import { Plus } from "lucide-react";
import NavChildLinks, {
  type NavChildLink,
} from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import { errorMessage } from "../../api/client";
import { toast } from "../../hooks/useToast";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useCheckin, useHabitsToday } from "./api";
import { formatAmount, todayProgress } from "./progress";

/**
 * 侧边栏“习惯”下面：今天（缩进列出每个习惯，右边“+”打一次卡）和健身。
 * 习惯多时只列前几个，没达标的排前面。
 */
const HABIT_LIMIT = 6;

export default function HabitsNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const today = useHabitsToday();
  const checkin = useCheckin();
  const list = [...(today.data ?? [])].sort(
    (a, b) => Number(a.reached) - Number(b.reached),
  );
  const p = todayProgress(today.data ?? []);
  const links: NavChildLink[] = [
    {
      key: "today",
      to: "/habits",
      label: t("Today"),
      hint: p.total ? `${p.reached}/${p.total}` : undefined,
    },
    ...list.slice(0, HABIT_LIMIT).map((h) => ({
      key: `h${h.habit.id}`,
      to: `/habits?habit=${h.habit.id}`,
      label: `${h.habit.icon ? h.habit.icon + " " : ""}${h.habit.name}`,
      hint: `${formatAmount(h.done)}/${formatAmount(h.habit.dailyTarget)}`,
      nested: true,
      active: false,
      action: (
        <button
          type="button"
          className="nav-child-action"
          title={`${t("Check in")}：${h.habit.name}`}
          aria-label={`${t("Check in")}：${h.habit.name}`}
          disabled={checkin.isPending}
          onClick={() =>
            checkin.mutate(
              { id: h.habit.id },
              {
                onSuccess: () => toast(`${h.habit.name} +1`),
                onError: (e) =>
                  toast({ message: errorMessage(e), tone: "error" }),
              },
            )
          }
        >
          <Plus size={13} />
        </button>
      ),
    })),
    { key: "fitness", to: "/habits/fitness", label: t("Fitness") },
    { key: "plan", to: "/habits/plan", label: t("Personal plan") },
  ];
  return (
    <NavChildLinks
      links={links}
      limit={links.length}
      allTo="/habits"
      loading={today.isPending}
      error={today.isError}
      empty={t("No habits yet")}
      onNavigate={onNavigate}
    />
  );
}
