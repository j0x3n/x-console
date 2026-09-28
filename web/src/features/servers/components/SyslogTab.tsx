import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError, isNotLive, unwrap, wsUrl } from "../../../api/client";
import LogViewer, { type LogLine } from "../../../components/log/LogViewer";
import {
  levelFromPriority,
  type LevelFilter,
} from "../../../components/log/levels";
import Switch from "../../../components/ui/Switch";
import { ErrorState, Loading, NotLive } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { hostsApi, type HostDetail } from "../api";
import type { components } from "../../../api/gen/hosts";

type Entry = components["schemas"]["SyslogEntry"];

/** 级别筛选换成 journal 的 priority 上限。 */
const priorityOf: Record<LevelFilter, number | undefined> = {
  all: undefined,
  error: 3,
  warn: 4,
  info: 6,
  debug: 7,
};

type Range = "15m" | "1h" | "today" | "7d";
function sinceOf(range: Range): string {
  const now = new Date();
  if (range === "today") {
    now.setHours(0, 0, 0, 0);
    return now.toISOString();
  }
  const minutes = { "15m": 15, "1h": 60, "7d": 7 * 24 * 60 }[range];
  return new Date(now.getTime() - minutes * 60_000).toISOString();
}

let seq = 1;
function toLine(e: Entry): LogLine {
  return {
    id: seq++,
    text: e.message,
    level: levelFromPriority(e.priority),
    time: e.time,
    source: e.unit ? (e.pid ? `${e.unit}[${e.pid}]` : e.unit) : undefined,
  };
}

const MAX = 10_000;

/**
 * 服务器和电脑的“日志”标签（B29）：systemd journal 或 Windows 事件。
 * 滚到顶部加载更早的一页；打开“实时”后新日志自动追加。
 */
export default function SyslogTab({ host }: { host: HostDetail }) {
  const t = useT();
  const [level, setLevel] = useState<LevelFilter>("all");
  const [unit, setUnit] = useState("");
  const [range, setRange] = useState<Range>("1h");
  const [live, setLive] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [error, setError] = useState<unknown>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [units, setUnits] = useState<string[]>([]);
  const [retry, setRetry] = useState(0);
  const query = useMemo(
    () => ({
      priority: priorityOf[level],
      unit: unit || undefined,
    }),
    [level, unit],
  );
  const reqId = useRef(0);

  useEffect(() => {
    unwrap(
      hostsApi.GET("/hosts/{hostId}/syslog/units", {
        params: { path: { hostId: host.id } },
      }),
    )
      .then((r) => setUnits(r.items))
      .catch(() => setUnits([]));
  }, [host.id]);

  // 条件变了，重新取最近的一页。
  useEffect(() => {
    const id = ++reqId.current;
    setState("loading");
    unwrap(
      hostsApi.GET("/hosts/{hostId}/syslog", {
        params: {
          path: { hostId: host.id },
          query: { ...query, since: sinceOf(range), limit: 1000 },
        },
      }),
    ).then(
      (page) => {
        if (id !== reqId.current) return;
        setLines(page.items.map(toLine));
        setCursor(page.cursor);
        setState("ready");
      },
      (err) => {
        if (id !== reqId.current) return;
        setError(err);
        setState("error");
      },
    );
  }, [host.id, query, range, retry]);

  const loadOlder = useCallback(() => {
    if (!cursor || loadingOlder) return;
    setLoadingOlder(true);
    unwrap(
      hostsApi.GET("/hosts/{hostId}/syslog", {
        params: {
          path: { hostId: host.id },
          query: { ...query, since: sinceOf(range), limit: 500, cursor },
        },
      }),
    )
      .then((page) => {
        setLines((prev) => [...page.items.map(toLine), ...prev].slice(0, MAX));
        setCursor(page.cursor);
      })
      .catch(() => setCursor(undefined))
      .finally(() => setLoadingOlder(false));
  }, [cursor, loadingOlder, host.id, query, range]);

  // 实时：WebSocket 推新日志，追加到最后。
  useEffect(() => {
    if (!live || state !== "ready") return;
    const params = new URLSearchParams();
    if (query.priority !== undefined)
      params.set("priority", String(query.priority));
    if (query.unit) params.set("unit", query.unit);
    const ws = new WebSocket(
      wsUrl(`/hosts/${encodeURIComponent(host.id)}/syslog/follow?${params}`),
    );
    ws.onmessage = (event) => {
      if (typeof event.data !== "string") return;
      try {
        const items = JSON.parse(event.data) as Entry[];
        if (!Array.isArray(items)) return;
        setLines((prev) => [...prev, ...items.map(toLine)].slice(-MAX));
      } catch {
        /* 不是 JSON 就忽略 */
      }
    };
    ws.onclose = (event) => {
      if (event.code !== 1000) setLive(false);
    };
    return () => ws.close();
  }, [live, state, host.id, query]);

  if (state === "error") {
    if (isNotLive(error))
      return (
        <div className="xc-card">
          <NotLive name={t("System logs")} />
        </div>
      );
    if (error instanceof ApiError && error.code === "syslog_permission")
      return (
        <div className="xc-card servers-syslog-perm">
          <strong>{t("The agent cannot read system logs")}</strong>
          <p className="xc-muted">{error.message}</p>
        </div>
      );
    return <ErrorState error={error} onRetry={() => setRetry((n) => n + 1)} />;
  }

  return (
    <div className="xc-card">
      {state === "loading" && lines.length === 0 ? (
        <Loading />
      ) : (
        <LogViewer
          lines={lines}
          level={level}
          onLevelChange={setLevel}
          onReachTop={cursor ? loadOlder : undefined}
          loadingOlder={loadingOlder}
          height="min(66vh, 640px)"
          empty={t("No logs in this time range")}
          toolbarEnd={
            <>
              <select
                className="xc-select servers-syslog-unit"
                aria-label={t("Service")}
                value={unit}
                onChange={(e) => setUnit(e.target.value)}
              >
                <option value="">{t("All services")}</option>
                {units.map((u) => (
                  <option key={u} value={u}>
                    {u}
                  </option>
                ))}
              </select>
              <select
                className="xc-select"
                aria-label={t("Time range")}
                value={range}
                onChange={(e) => setRange(e.target.value as Range)}
              >
                <option value="15m">{t("Last 15 minutes")}</option>
                <option value="1h">{t("Last hour")}</option>
                <option value="today">{t("Today")}</option>
                <option value="7d">{t("Last 7 days")}</option>
              </select>
              <span className="servers-syslog-live">
                <Switch checked={live} onChange={setLive} label={t("Live")} />
                <span>{t("Live")}</span>
              </span>
            </>
          }
        />
      )}
    </div>
  );
}
