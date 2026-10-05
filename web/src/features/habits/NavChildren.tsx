import { useLocation } from "react-router";
import { ChartColumn, ClipboardList, Dumbbell, Plus, Sun } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
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
 * 左栏“习惯”的二级菜单（B102）：今天、健身、个人计划、统计（B103），
 * 下面列出今天的习惯，右边“+”打一次卡。没达标的排前面。
 */

export default function HabitsNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const today = useHabitsToday();
  const checkin = useCheckin();
  const list = [...(today.data ?? [])].sort(
    (a, b) => Number(a.reached) - Number(b.reached),
  );
  const p = todayProgress(today.data ?? []);
  const { pathname } = useLocation();
  const links: NavChildLink[] = [
    ...list.map((h) => ({
      key: `h${h.habit.id}`,
      to: `/habits?habit=${h.habit.id}`,
      label: `${h.habit.icon ? h.habit.icon + " " : ""}${h.habit.name}`,
      hint: `${formatAmount(h.done)}/${formatAmount(h.habit.dailyTarget)}`,
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
  ];
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/habits"
          icon={Sun}
          label={t("Today")}
          active={pathname === "/habits"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/habits/fitness"
          icon={Dumbbell}
          label={t("Fitness")}
          active={pathname.startsWith("/habits/fitness")}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/habits/plan"
          icon={ClipboardList}
          label={t("Personal plan")}
          active={pathname.startsWith("/habits/plan")}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/habits/stats"
          icon={ChartColumn}
          label={t("Stats")}
          active={pathname.startsWith("/habits/stats")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup
        label={`${t("Today")} ${p.total ? `${p.reached}/${p.total}` : ""}`.trim()}
      >
        <NavChildLinks
          links={links}
          limit={links.length}
          allTo="/habits"
          loading={today.isPending}
          error={today.isError}
          empty={t("No habits yet")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
