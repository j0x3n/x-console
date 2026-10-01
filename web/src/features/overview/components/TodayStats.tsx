import {
  Ring,
  Segments,
  StatCard,
  StatStrip,
} from "../../../components/ui/Stat";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatTime } from "../../../lib/time";
import { useCalendarEvents } from "../../calendar/api";
import { addDays, startOfDay } from "../../calendar/dates";
import { useTasks } from "../../coding/api";
import { useHabitsToday } from "../../habits/api";
import { useHosts } from "../../servers/api";
import { nextEvent } from "../today";
import { useTodoCount } from "./MainCards";

const DASH = "–";

/** 顶部一排概要：待办、提醒、习惯、服务器、Agent 任务、下一项日程。 */
export default function TodayStats() {
  const t = useT();
  const language = useLanguage();
  const todo = useTodoCount();
  const habits = useHabitsToday();
  const hosts = useHosts("server");
  const tasks = useTasks(["queued", "running", "review"]);
  const from = startOfDay(new Date());
  const events = useCalendarEvents(from, addDays(from, 1));

  const habitTotal = habits.data?.length ?? 0;
  const habitDone = habits.data?.filter((h) => h.reached).length ?? 0;
  const bestStreak = Math.max(0, ...(habits.data ?? []).map((h) => h.streak));

  const servers = hosts.data ?? [];
  const online = servers.filter((h) => h.online).length;
  const alerts = servers.reduce((sum, h) => sum + h.activeAlerts, 0);

  const running = tasks.data?.filter((x) => x.status === "running").length ?? 0;
  const queued = tasks.data?.filter((x) => x.status === "queued").length ?? 0;
  const review = tasks.data?.filter((x) => x.status === "review").length ?? 0;

  const next = events.data ? nextEvent(events.data, new Date()) : undefined;

  return (
    <StatStrip size="large" label={t("Today at a glance")}>
      <StatCard
        label={t("To do today")}
        to="/projects"
        value={todo.issues ?? DASH}
        caption={
          todo.overdue ? `${t("Overdue by")} ${todo.overdue}` : undefined
        }
        tone={todo.overdue ? "warn" : undefined}
        foot={t("Issues due today")}
      />
      <StatCard
        label={t("Reminders")}
        to="/reminders"
        value={todo.reminders ?? DASH}
        foot={t("Reminders for today")}
      />
      <StatCard
        label={t("Habits")}
        to="/habits"
        caption={bestStreak > 0 ? `${bestStreak} ${t("days")}` : undefined}
        foot={
          habitTotal
            ? `${habitDone}/${habitTotal} ${t("goals reached")}`
            : t("No habits yet")
        }
      >
        <div className="today-stat-ring">
          <Ring
            value={habitDone}
            max={habitTotal}
            size={46}
            tone={habitTotal > 0 && habitDone === habitTotal ? "ok" : "accent"}
          >
            {habitTotal
              ? `${Math.round((habitDone / habitTotal) * 100)}%`
              : DASH}
          </Ring>
        </div>
      </StatCard>
      <StatCard
        label={t("Servers")}
        to="/servers"
        value={hosts.data ? online : DASH}
        unit={hosts.data ? `/ ${servers.length} ${t("online")}` : undefined}
        tone={
          servers.length > online ? "danger" : alerts > 0 ? "warn" : undefined
        }
        caption={alerts ? `${alerts} ${t("alerts")}` : undefined}
        foot={
          servers.length === 0 && hosts.data ? t("No servers yet") : undefined
        }
      >
        {servers.length > 0 && (
          <Segments
            parts={[
              { value: online, tone: "ok" },
              { value: servers.length - online, tone: "danger" },
            ]}
          />
        )}
      </StatCard>
      <StatCard
        label={t("Coding tasks")}
        to="/coding/tasks"
        value={tasks.data ? running : DASH}
        unit={tasks.data ? t("running") : undefined}
        caption={review ? `${review} ${t("to review")}` : undefined}
        tone={review ? "accent" : undefined}
        foot={`${queued} ${t("queued")}`}
      />
      <StatCard
        label={t("Next event")}
        to="/calendar"
        value={next ? formatTime(next.start, language) : DASH}
        foot={next ? next.title : t("Nothing else today")}
      />
    </StatStrip>
  );
}
