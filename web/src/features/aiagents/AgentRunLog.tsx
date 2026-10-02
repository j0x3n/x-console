import { useMemo, useState } from "react";
import { Link } from "react-router";
import {
  AlertTriangle,
  CircleHelp,
  ExternalLink,
  MessageSquareText,
  Square,
  Wrench,
} from "lucide-react";
import { isNotLive } from "../../api/client";
import { EmptyState, Loading, NotLive } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useCancelTask, useTaskEvents, useTasks } from "../coding/api";
import OutputView from "../coding/components/OutputView";
import {
  useAgentRunEvents,
  useAgentRuns,
  useCancelAgentRun,
  type AiAgentRunEvent,
} from "./api";
import { runEntries, runStatusLabel, runTone, type RunEntry } from "./logic";
import "./i18n";
import "./aiagents.css";

/**
 * B86：Agent 的执行日志。Agent 详情页按 Agent 看，卡片详情按卡片看。
 * 执行记录接口还没上线时，只用编码任务（CLI 类型）的数据。
 */
export default function AgentRunLog({
  agentId,
  issueKey,
  hideWhenEmpty,
}: {
  agentId?: number;
  issueKey?: string;
  /** 卡片详情里没有执行记录时整块不显示 */
  hideWhenEmpty?: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const runs = useAgentRuns({ agentId, issueKey });
  const tasks = useTasks();
  const live = !(runs.isError && isNotLive(runs.error));
  const entries = useMemo(
    () =>
      runEntries({
        runs: runs.data,
        tasks: (tasks.data ?? []).filter((x) =>
          issueKey
            ? x.issueKey === issueKey && x.aiAgentId != null
            : x.aiAgentId === agentId,
        ),
      }),
    [runs.data, tasks.data, agentId, issueKey],
  );
  const [picked, setPicked] = useState<string>();
  const current =
    entries.find((e) => e.key === picked) ??
    entries.find((e) => e.active) ??
    entries[0];

  if (runs.isPending && tasks.isPending)
    return hideWhenEmpty ? null : <Loading />;
  if (entries.length === 0) {
    if (hideWhenEmpty) return null;
    return (
      <EmptyState
        title={t("No agent runs yet")}
        icon={<MessageSquareText size={24} />}
      >
        <span className="xc-muted">
          {t("Logs show up here after the agent starts working.")}
        </span>
      </EmptyState>
    );
  }
  return (
    <div className="aiagent-runlog">
      <div className="aiagent-runlog-head">
        <select
          className="xc-select"
          aria-label={t("Pick a run")}
          value={current?.key}
          onChange={(e) => setPicked(e.target.value)}
        >
          {entries.map((e) => (
            <option key={e.key} value={e.key}>
              {[
                issueKey ? e.agentName : e.issueKey,
                e.title,
                t(runStatusLabel(e.status)),
                relativeTime(e.at, language),
              ]
                .filter(Boolean)
                .join(" · ")}
            </option>
          ))}
        </select>
        {current && (
          <span className={`xc-badge ${runTone(current.status)}`}>
            {t(runStatusLabel(current.status))}
          </span>
        )}
        {current?.taskId && (
          <Link className="xc-btn small ghost" to={`/coding/${current.taskId}`}>
            <ExternalLink size={13} /> {t("Open task")}
          </Link>
        )}
        {current?.prUrl && (
          <a
            className="xc-btn small ghost"
            href={current.prUrl}
            target="_blank"
            rel="noreferrer"
          >
            <ExternalLink size={13} /> PR
          </a>
        )}
        {current?.active && <StopButton entry={current} />}
      </div>
      {current?.summary && !current.active && (
        <p className="aiagent-runlog-summary">{current.summary}</p>
      )}
      {current &&
        (current.taskId ? (
          <TaskLog taskId={current.taskId} running={current.active} />
        ) : live ? (
          <RunEvents runId={current.runId} />
        ) : (
          <NotLive name="执行日志" />
        ))}
    </div>
  );
}

function StopButton({ entry }: { entry: RunEntry }) {
  const t = useT();
  const cancelRun = useCancelAgentRun();
  const cancelTask = useCancelTask();
  const busy = cancelRun.isPending || cancelTask.isPending;
  return (
    <button
      className="xc-btn small danger"
      disabled={busy}
      onClick={() => {
        if (entry.runId) cancelRun.mutate(entry.runId);
        else if (entry.taskId) cancelTask.mutate(entry.taskId);
      }}
    >
      <Square size={12} /> {t("Stop")}
    </button>
  );
}

/** CLI 类型：直接用编码任务的输出。 */
function TaskLog({ taskId, running }: { taskId: number; running: boolean }) {
  const events = useTaskEvents(taskId);
  if (events.isPending) return <Loading />;
  if (events.isError) return null;
  return (
    <div className="aiagent-runlog-body">
      <OutputView events={events.data} running={running} />
    </div>
  );
}

/** 内置类型：工具调用、输出、出错，一行一条。 */
function RunEvents({ runId }: { runId?: number }) {
  const t = useT();
  const language = useLanguage();
  const events = useAgentRunEvents(runId);
  if (!runId) return null;
  if (events.isPending) return <Loading />;
  if (events.isError)
    return isNotLive(events.error) ? <NotLive name="执行日志" /> : null;
  const list = Array.isArray(events.data) ? events.data : [];
  if (list.length === 0)
    return <p className="xc-muted aiagent-runlog-summary">{t("No log yet")}</p>;
  return (
    <ol className="aiagent-runlog-body aiagent-events">
      {list.map((e) => (
        <li
          key={e.seq}
          className={`is-${e.kind}${e.ok === false ? " is-failed" : ""}`}
        >
          <EventIcon event={e} />
          <span className="aiagent-event-text">{e.text}</span>
          <time dateTime={e.at} title={e.at}>
            {relativeTime(e.at, language)}
          </time>
        </li>
      ))}
    </ol>
  );
}

function EventIcon({ event }: { event: AiAgentRunEvent }) {
  switch (event.kind) {
    case "tool":
      return <Wrench size={13} />;
    case "error":
      return <AlertTriangle size={13} />;
    case "decision":
      return <CircleHelp size={13} />;
    default:
      return <MessageSquareText size={13} />;
  }
}
