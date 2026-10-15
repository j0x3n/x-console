import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ErrorState, EmptyState, Loading } from "../../components/ui/States";
import { SearchBox, Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import type { Language } from "../../types/domain";
import { formatDate, formatTime } from "../../lib/time";
import {
  routerKeys,
  useRouterDevices,
  useRouterEvents,
  useRouterLogs,
  type EventGroup,
  type LogLevel,
  type RouterEvent,
  type RouterLogLine,
} from "./api";

type Part = "events" | "system";
const PART_KEY = "router.logs.part";

function readPart(): Part {
  try {
    return localStorage.getItem(PART_KEY) === "system" ? "system" : "events";
  } catch {
    return "events";
  }
}

/** 时间：今天的只写时分秒，别的天前面加日期。 */
function stamp(at: string, language: Language) {
  const d = new Date(at);
  const time = d.toLocaleTimeString(language === "zh" ? "zh-CN" : "en", {
    hour12: false,
  });
  return d.toDateString() === new Date().toDateString()
    ? time
    : `${formatDate(d, language)} ${time}`;
}

/** B230：路由器页的日志页签。事件是面板记的，系统日志是路由器的 logread。 */
export default function LogsView({
  onOpenDevice,
}: {
  onOpenDevice: (mac: string) => void;
}) {
  const t = useT();
  const [part, setPartState] = useState<Part>(readPart);
  const setPart = (next: Part) => {
    setPartState(next);
    try {
      localStorage.setItem(PART_KEY, next);
    } catch {
      /* 存不了就算了 */
    }
  };
  const switcher = (
    <Segmented
      label={t("Logs")}
      value={part}
      onChange={setPart}
      options={[
        { value: "events", label: t("Events") },
        { value: "system", label: t("System log") },
      ]}
    />
  );
  return part === "events" ? (
    <EventsPart switcher={switcher} onOpenDevice={onOpenDevice} />
  ) : (
    <SystemPart switcher={switcher} />
  );
}

const GROUPS: Array<{ value: EventGroup | ""; label: string }> = [
  { value: "", label: "All" },
  { value: "device", label: "Devices" },
  { value: "wan", label: "WAN" },
  { value: "report", label: "Reports" },
  { value: "command", label: "Commands" },
];

/** 事件类型标签的颜色。 */
const KIND_TONE: Record<string, string> = {
  device_new: "accent",
  device_online: "ok",
  device_offline: "",
  devices_seeded: "info",
  wan_down: "danger",
  wan_up: "ok",
  report_lost: "danger",
  report_back: "ok",
  interface_restart: "warn",
  reboot: "warn",
};

const KIND_LABEL: Record<string, string> = {
  device_new: "New device",
  device_online: "Online",
  device_offline: "Offline",
  devices_seeded: "First report",
  wan_down: "WAN down",
  wan_up: "WAN up",
  report_lost: "No reports",
  report_back: "Reports back",
  interface_restart: "Restart interface",
  reboot: "Reboot router",
};

function EventsPart({
  switcher,
  onOpenDevice,
}: {
  switcher: React.ReactNode;
  onOpenDevice: (mac: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const [group, setGroup] = useState<EventGroup | "">("");
  const [mac, setMac] = useState("");
  const devices = useRouterDevices();
  const events = useRouterEvents(group, mac);
  const items = events.data?.pages.flatMap((p) => p.items) ?? [];
  const names = new Map(
    (devices.data?.items ?? []).map((d) => [d.mac, d.displayName]),
  );

  let body;
  if (events.isPending) body = <Loading />;
  else if (events.isError)
    body = <ErrorState error={events.error} onRetry={() => events.refetch()} />;
  else if (items.length === 0) body = <EmptyState title={t("No events yet")} />;
  else
    body = (
      <ul className="router-log-list">
        {items.map((e) => (
          <EventRow
            key={e.id}
            e={e}
            language={language}
            onOpenDevice={onOpenDevice}
          />
        ))}
      </ul>
    );

  return (
    <>
      <Toolbar
        start={switcher}
        end={
          <>
            <select
              className="xc-select router-log-select"
              aria-label={t("Event type")}
              value={group}
              onChange={(e) => setGroup(e.target.value as EventGroup | "")}
            >
              {GROUPS.map((g) => (
                <option key={g.value} value={g.value}>
                  {t(g.label)}
                </option>
              ))}
            </select>
            {devices.data && devices.data.items.length > 0 && (
              <select
                className="xc-select router-log-select"
                aria-label={t("Device")}
                value={mac}
                onChange={(e) => setMac(e.target.value)}
              >
                <option value="">{t("All devices")}</option>
                {devices.data.items.map((d) => (
                  <option key={d.mac} value={d.mac}>
                    {names.get(d.mac)}
                  </option>
                ))}
              </select>
            )}
          </>
        }
      />
      <LogFrame
        pages={events.data?.pages.length ?? 0}
        onLatest={() =>
          qc.resetQueries({ queryKey: routerKeys.events(group, mac) })
        }
        hasMore={events.hasNextPage}
        loadingMore={events.isFetchingNextPage}
        onMore={() => events.fetchNextPage()}
      >
        {body}
      </LogFrame>
    </>
  );
}

function EventRow({
  e,
  language,
  onOpenDevice,
}: {
  e: RouterEvent;
  language: Language;
  onOpenDevice: (mac: string) => void;
}) {
  const t = useT();
  const tone = KIND_TONE[e.kind] ?? "";
  const label = KIND_LABEL[e.kind];
  const content = (
    <>
      <time className="router-log-time">{stamp(e.at, language)}</time>
      <span className={`xc-badge ${tone}`}>{label ? t(label) : e.kind}</span>
      <span className="router-log-text">
        {e.title}
        {e.detail && <small> {e.detail}</small>}
      </span>
    </>
  );
  return (
    <li className="router-log-row">
      {e.mac ? (
        <button
          type="button"
          className="router-log-link"
          onClick={() => onOpenDevice(e.mac!)}
        >
          {content}
        </button>
      ) : (
        content
      )}
    </li>
  );
}

function SystemPart({ switcher }: { switcher: React.ReactNode }) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const [q, setQ] = useState("");
  const [query, setQuery] = useState("");
  const [level, setLevel] = useState<LogLevel | "">("");
  const [program, setProgram] = useState("");
  // 停下输入半秒再搜，免得每个字都请求一次
  useEffect(() => {
    const id = window.setTimeout(() => setQuery(q.trim()), 400);
    return () => window.clearTimeout(id);
  }, [q]);
  const logs = useRouterLogs(query, level, program);
  const items = logs.data?.pages.flatMap((p) => p.items) ?? [];
  const programs = logs.data?.pages[0]?.programs ?? [];

  let body;
  if (logs.isPending) body = <Loading />;
  else if (logs.isError)
    body = <ErrorState error={logs.error} onRetry={() => logs.refetch()} />;
  else if (items.length === 0)
    body = (
      <EmptyState title={t("No system log yet")}>
        <span>
          路由器要用新版脚本才会上报系统日志。没有的话，在设置里重新生成令牌，再执行一次安装命令。
        </span>
      </EmptyState>
    );
  else
    body = (
      <ul className="router-log-list is-system">
        {items.map((l) => (
          <SystemRow key={l.id} l={l} language={language} />
        ))}
      </ul>
    );

  return (
    <>
      <Toolbar
        start={switcher}
        end={
          <>
            <select
              className="xc-select router-log-select"
              aria-label={t("Level")}
              value={level}
              onChange={(e) => setLevel(e.target.value as LogLevel | "")}
            >
              <option value="">{t("All levels")}</option>
              <option value="warn">{t("Warnings and above")}</option>
              <option value="err">{t("Errors and above")}</option>
            </select>
            <select
              className="xc-select router-log-select"
              aria-label={t("Program")}
              value={program}
              onChange={(e) => setProgram(e.target.value)}
            >
              <option value="">{t("All programs")}</option>
              {programs.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search the log")}
            />
          </>
        }
      />
      <LogFrame
        pages={logs.data?.pages.length ?? 0}
        onLatest={() =>
          qc.resetQueries({ queryKey: routerKeys.logs(query, level, program) })
        }
        hasMore={logs.hasNextPage}
        loadingMore={logs.isFetchingNextPage}
        onMore={() => logs.fetchNextPage()}
      >
        {body}
      </LogFrame>
    </>
  );
}

/** 错误及以上标红，警告标黄。 */
function levelTone(level: string) {
  if (["emerg", "alert", "crit", "err"].includes(level)) return "is-err";
  if (level === "warn") return "is-warn";
  return "";
}

function SystemRow({ l, language }: { l: RouterLogLine; language: Language }) {
  return (
    <li className={`router-log-row ${levelTone(l.level)}`}>
      <time className="router-log-time">{stamp(l.at, language)}</time>
      <span className="router-log-level">{l.level || "—"}</span>
      <span className="router-log-text">
        {l.program && <b>{l.program}</b>} {l.message}
      </span>
    </li>
  );
}

/** 翻过页以后不自动刷新，顶上给一个回到最新的按钮；底部加载更多。 */
function LogFrame({
  pages,
  onLatest,
  hasMore,
  loadingMore,
  onMore,
  children,
}: {
  pages: number;
  onLatest: () => void;
  hasMore: boolean;
  loadingMore: boolean;
  onMore: () => void;
  children: React.ReactNode;
}) {
  const t = useT();
  return (
    <div className="xc-card router-logs">
      {pages > 1 && (
        <div className="router-log-paused">
          <span>{t("Paused while you read older lines.")}</span>
          <button type="button" className="xc-btn small" onClick={onLatest}>
            {t("Show the latest")}
          </button>
        </div>
      )}
      {children}
      {hasMore && (
        <div className="router-log-more">
          <button
            type="button"
            className="xc-btn small"
            disabled={loadingMore}
            onClick={onMore}
          >
            {t("Load more")}
          </button>
        </div>
      )}
    </div>
  );
}
