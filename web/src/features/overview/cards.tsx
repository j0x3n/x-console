import { useQuery } from "@tanstack/react-query";
import type { ComponentType } from "react";
import { Link } from "react-router";
import { ApiError, errorMessage, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { toast } from "../../hooks/useToast";
import { briefApi, calendarKeys, useCalendarEvents } from "../calendar/api";
import { useTasks } from "../coding/api";
import { useCheckin, useHabitsToday } from "../habits/api";
import {
  useCallService,
  useHAEvents,
  useHAFavorites,
  useHAStatus,
} from "../home/api";
import { displayName, stateLabel, tapAction } from "../home/logic";
import { projectsApi } from "../projects/api";
import { useReminders } from "../reminders/api";
import { useHosts } from "../servers/api";

export type CardID =
  | "greeting"
  | "issues"
  | "reminders"
  | "habits"
  | "servers"
  | "coding"
  | "weather"
  | "home"
  | "calendar";

export const cardLabels: Record<CardID, string> = {
  greeting: "Overview greeting",
  issues: "Overview issues",
  reminders: "Overview reminders",
  habits: "Overview habits",
  servers: "Overview servers",
  coding: "Overview coding",
  weather: "Overview weather",
  home: "Overview home",
  calendar: "Overview calendar",
};

function LoadingCard() {
  return <p className="overview-empty">正在加载…</p>;
}
function EmptyCard({
  text,
  to,
  link,
}: {
  text: string;
  to?: string;
  link?: string;
}) {
  return (
    <p className="overview-empty">
      {text} {to && <Link to={to}>{link ?? "查看"}</Link>}
    </p>
  );
}
function FailedCard({ error, retry }: { error: unknown; retry: () => void }) {
  return (
    <div className="overview-error">
      <span>{errorMessage(error)}</span>
      <button className="xc-btn small" onClick={retry}>
        重试
      </button>
    </div>
  );
}
function timeText(value?: string) {
  if (!value) return "";
  return new Date(value).toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function GreetingCard() {
  const now = new Date();
  const hour = now.getHours();
  const greeting =
    hour < 6
      ? "夜深了"
      : hour < 11
        ? "早上好"
        : hour < 14
          ? "中午好"
          : hour < 18
            ? "下午好"
            : "晚上好";
  return (
    <>
      <p className="overview-greeting-text">{greeting}</p>
      <p className="overview-summary">
        {new Intl.DateTimeFormat("zh-CN", { dateStyle: "full" }).format(now)}
      </p>
    </>
  );
}

const todayIssuesKey = ["overview", "issues", "today"] as const;
invalidateOn("issue.", todayIssuesKey);

function IssuesCard() {
  const issues = useQuery({
    queryKey: todayIssuesKey,
    queryFn: () =>
      unwrap(
        projectsApi.GET("/issues", {
          params: {
            query: {
              due: "today",
              status: ["backlog", "todo", "in_progress", "in_review"],
              sort: "due",
              limit: 10,
            },
          },
        }),
      ),
  });
  if (issues.isPending) return <LoadingCard />;
  if (issues.isError)
    return (
      <FailedCard error={issues.error} retry={() => void issues.refetch()} />
    );
  if (!issues.data.items.length)
    return (
      <EmptyCard text="今天没有到期的待办。" to="/projects" link="查看项目" />
    );
  return (
    <ul className="overview-list">
      {issues.data.items.map((issue) => (
        <li key={issue.id}>
          <Link to={`/projects/${issue.projectKey}/${issue.number}`}>
            {issue.title}
          </Link>
          <small>{issue.key}</small>
        </li>
      ))}
    </ul>
  );
}

function RemindersCard() {
  const reminders = useReminders("upcoming");
  if (reminders.isPending) return <LoadingCard />;
  if (reminders.isError)
    return (
      <FailedCard
        error={reminders.error}
        retry={() => void reminders.refetch()}
      />
    );
  if (!reminders.data.length)
    return (
      <EmptyCard text="暂无即将到期的提醒。" to="/reminders" link="新建提醒" />
    );
  return (
    <ul className="overview-list">
      {reminders.data.slice(0, 5).map((item) => (
        <li key={item.id}>
          <Link to="/reminders">{item.title}</Link>
          <small>{timeText(item.dueAt ?? item.nextAt)}</small>
        </li>
      ))}
    </ul>
  );
}

function HabitsCard() {
  const today = useHabitsToday();
  const checkin = useCheckin();
  if (today.isPending) return <LoadingCard />;
  if (today.isError)
    return (
      <FailedCard error={today.error} retry={() => void today.refetch()} />
    );
  if (!today.data.length)
    return <EmptyCard text="还没有习惯。" to="/habits" link="去添加" />;
  const reached = today.data.filter((item) => item.reached).length;
  return (
    <>
      <p className="overview-summary">
        今天已完成 {reached}/{today.data.length}
      </p>
      <ul className="overview-list">
        {today.data.slice(0, 5).map((item) => (
          <li key={item.habit.id}>
            <Link to="/habits">
              {item.habit.icon} {item.habit.name}
            </Link>
            <button
              className="xc-btn small"
              disabled={checkin.isPending}
              onClick={() =>
                checkin.mutate(
                  { id: item.habit.id },
                  {
                    onSuccess: () => toast("已打卡"),
                    onError: (error) =>
                      toast({ message: errorMessage(error), tone: "error" }),
                  },
                )
              }
            >
              {item.reached ? "再记一次" : "打卡"}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}

function ServersCard() {
  const hosts = useHosts("server");
  if (hosts.isPending) return <LoadingCard />;
  if (hosts.isError)
    return (
      <FailedCard error={hosts.error} retry={() => void hosts.refetch()} />
    );
  if (!hosts.data.length)
    return (
      <EmptyCard text="还没有服务器。" to="/settings/devices" link="添加代理" />
    );
  const sorted = [...hosts.data].sort(
    (a, b) =>
      Number(a.online) - Number(b.online) || b.activeAlerts - a.activeAlerts,
  );
  return (
    <>
      <p className="overview-summary">
        {hosts.data.filter((host) => host.online).length}/{hosts.data.length}{" "}
        台在线
      </p>
      <ul className="overview-list">
        {sorted.slice(0, 5).map((host) => (
          <li key={host.id}>
            <Link to={`/servers/${host.id}`}>{host.name}</Link>
            <small>
              {!host.online
                ? "离线"
                : host.activeAlerts
                  ? `${host.activeAlerts} 条告警`
                  : "正常"}
            </small>
          </li>
        ))}
      </ul>
    </>
  );
}

function CodingCard() {
  const tasks = useTasks(["running", "review"]);
  if (tasks.isPending) return <LoadingCard />;
  if (tasks.isError)
    return (
      <FailedCard error={tasks.error} retry={() => void tasks.refetch()} />
    );
  if (!tasks.data.length)
    return (
      <EmptyCard
        text="没有运行中或待处理的编码任务。"
        to="/coding"
        link="查看任务"
      />
    );
  return (
    <ul className="overview-list">
      {tasks.data.slice(0, 5).map((task) => (
        <li key={task.id}>
          <Link to={`/coding/${task.id}`}>{task.title || task.repoName}</Link>
          <small>{task.status === "review" ? "待决定" : "运行中"}</small>
        </li>
      ))}
    </ul>
  );
}

function WeatherCard() {
  const weather = useQuery({
    queryKey: calendarKeys.weather,
    queryFn: () => unwrap(briefApi.GET("/weather")),
  });
  if (weather.isPending) return <LoadingCard />;
  if (weather.isError) {
    if (
      weather.error instanceof ApiError &&
      (weather.error.status === 412 ||
        weather.error.code === "integration_not_configured")
    )
      return (
        <EmptyCard
          text="设置位置后显示天气。"
          to="/settings/brief"
          link="去设置"
        />
      );
    return (
      <FailedCard error={weather.error} retry={() => void weather.refetch()} />
    );
  }
  return (
    <>
      <p className="overview-greeting-text">{weather.data.temperature}°C</p>
      <p className="overview-summary">
        {weather.data.location || weather.data.summary} · {weather.data.summary}
      </p>
      <p className="overview-empty">
        最高 {weather.data.high}° · 最低 {weather.data.low}° · 降水概率{" "}
        {weather.data.precipitationChance}%
      </p>
    </>
  );
}

function HomeCard() {
  useHAEvents();
  const status = useHAStatus();
  const favorites = useHAFavorites();
  const call = useCallService();
  if (status.isPending || favorites.isPending) return <LoadingCard />;
  if (status.isError)
    return (
      <FailedCard error={status.error} retry={() => void status.refetch()} />
    );
  if (!status.data.configured)
    return (
      <EmptyCard
        text="还没有连接 Home Assistant。"
        to="/settings/homeassistant"
        link="去设置"
      />
    );
  if (favorites.isError)
    return (
      <FailedCard
        error={favorites.error}
        retry={() => void favorites.refetch()}
      />
    );
  if (!favorites.data.length)
    return <EmptyCard text="还没有收藏的设备。" to="/home" link="去收藏" />;
  return (
    <ul className="overview-list">
      {favorites.data.slice(0, 5).map((favorite) => {
        const action = tapAction(favorite.entityId, favorite.state);
        return (
          <li key={favorite.entityId}>
            <Link to="/home">
              {displayName(favorite.entityId, favorite.state, favorite.alias)}
            </Link>
            {action.kind === "toggle" ? (
              <button
                className="xc-btn small"
                disabled={call.isPending || !status.data.connected}
                onClick={() =>
                  call.mutate(
                    {
                      domain: action.domain,
                      service: action.service,
                      entityId: favorite.entityId,
                    },
                    {
                      onError: (error) =>
                        toast({ message: errorMessage(error), tone: "error" }),
                    },
                  )
                }
              >
                {favorite.state?.state === "on" ? "关闭" : "开启"}
              </button>
            ) : (
              <small>
                {favorite.state ? stateLabel(favorite.state) : "未知"}
              </small>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function CalendarCard() {
  const from = new Date();
  from.setHours(0, 0, 0, 0);
  const to = new Date(from);
  to.setDate(to.getDate() + 1);
  const events = useCalendarEvents(from, to);
  if (events.isPending) return <LoadingCard />;
  if (events.isError)
    return (
      <FailedCard error={events.error} retry={() => void events.refetch()} />
    );
  if (!events.data.length)
    return <EmptyCard text="今天没有日程。" to="/calendar" link="查看日历" />;
  return (
    <ul className="overview-list">
      {events.data.slice(0, 5).map((event) => (
        <li key={event.id}>
          <Link to="/calendar">{event.title}</Link>
          <small>{event.allDay ? "全天" : timeText(event.start)}</small>
        </li>
      ))}
    </ul>
  );
}

export const cardComponents: Record<CardID, ComponentType> = {
  greeting: GreetingCard,
  issues: IssuesCard,
  reminders: RemindersCard,
  habits: HabitsCard,
  servers: ServersCard,
  coding: CodingCard,
  weather: WeatherCard,
  home: HomeCard,
  calendar: CalendarCard,
};
