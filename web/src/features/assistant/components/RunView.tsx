import { useEffect, useRef } from "react";
import { ExternalLink } from "lucide-react";
import { EmptyState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { useAgentDecisions, useAgentRuns } from "../../aiagents/api";
import { RunEvents, StopButton, TaskLog } from "../../aiagents/AgentRunLog";
import { DecisionRow } from "../../aiagents/DecisionRows";
import { runEntries } from "../../aiagents/logic";
import type { AiSession } from "../api";

/**
 * B183：AI 窗口右边的一次 Agent 执行。只看日志，可以回答它的问题、批准、停止，不能聊天。
 * 日志跟 Agent 详情页同一套数据，有新日志时自动刷新。
 */
export default function RunView({
  runId,
  session,
}: {
  runId: number;
  session: AiSession | undefined;
}) {
  const t = useT();
  const runs = useAgentRuns({ agentId: session?.agentId });
  const decisions = useAgentDecisions();
  const run = runs.data?.find((r) => r.id === runId);
  const entry = run ? runEntries({ runs: [run], tasks: [] })[0] : undefined;
  const mine = (decisions.data ?? []).filter((d) => d.runId === runId);
  const bodyRef = useRef<HTMLDivElement>(null);

  // 日志变长时跟着滚到底。
  useEffect(() => {
    const el = bodyRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => (el.scrollTop = el.scrollHeight));
    if (el.firstElementChild) ro.observe(el.firstElementChild);
    return () => ro.disconnect();
  }, [runId, !!entry]);

  if (!session || runs.isPending)
    return (
      <div className="ai-body">
        <Loading />
      </div>
    );
  if (!run || !entry)
    return (
      <div className="ai-body">
        <EmptyState title={t("Run not found")} />
      </div>
    );
  return (
    <>
      <div className="ai-body ai-run-body" ref={bodyRef}>
        <div className="ai-run">
          {entry.summary && !entry.active && (
            <p className="ai-run-summary">{entry.summary}</p>
          )}
          {mine.length > 0 && (
            <div className="ai-run-decisions">
              {mine.map((d) => (
                <DecisionRow key={d.id} decision={d} />
              ))}
            </div>
          )}
          {entry.taskId ? (
            <TaskLog taskId={entry.taskId} running={entry.active} />
          ) : (
            <RunEvents runId={entry.runId} />
          )}
        </div>
      </div>
      <div className="ai-run-foot">
        <span className="ai-muted">
          {t("This is an agent run. You can approve, reject or stop it here.")}
        </span>
        {entry.prUrl && (
          <a
            className="xc-btn small ghost"
            href={entry.prUrl}
            target="_blank"
            rel="noreferrer"
          >
            <ExternalLink size={13} /> PR
          </a>
        )}
        {entry.active && <StopButton entry={entry} />}
      </div>
    </>
  );
}
