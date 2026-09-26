import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ChevronRight, CircleCheck, CircleX, Info, Loader2, Wrench } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { Translate } from "../../../types/domain";
import { buildBlocks, type OutputBlock, type TaskEvent, type ToolCall } from "../logic";

/** 状态事件的说明，按 data.code 翻译。 */
function statusText(block: Extract<OutputBlock, { type: "status" }>, t: Translate): string {
  const d = block.data;
  switch (block.code) {
    case "worktree_ready":
      return `${t("Worktree ready on branch")} ${String(d.branch ?? "")}`;
    case "started":
      return t("Executor started");
    case "session_started":
      return d.model ? `${t("Session started")} · ${String(d.model)}` : t("Session started");
    case "result": {
      const parts = [t("Executor finished")];
      if (typeof d.turns === "number" && d.turns > 0) parts.push(`${d.turns} ${t("turns")}`);
      if (typeof d.costUsd === "number" && d.costUsd > 0) parts.push(`$${d.costUsd.toFixed(2)}`);
      return parts.join(" · ");
    }
    case "canceled":
      return t("Stopped on request");
    case "timeout":
      return t("Stopped after the time limit");
    case "plan":
      return t("Plan updated");
  }
  return block.text;
}

function doneText(block: Extract<OutputBlock, { type: "done" }>, t: Translate): string {
  switch (block.reason) {
    case "exited":
      return `${t("Exited with code")} ${block.exitCode ?? "?"}`;
    case "timeout":
      return t("Timed out");
    case "canceled":
      return t("Canceled");
    case "start_failed":
      return t("Could not start");
  }
  return block.reason;
}

function ToolRow({ call }: { call: ToolCall }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const input = call.input === undefined ? "" : JSON.stringify(call.input, null, 2);
  return (
    <li className={`coding-tool${call.isError ? " error" : ""}`}>
      <button type="button" className="coding-tool-head" aria-expanded={open} onClick={() => setOpen(!open)}>
        <ChevronRight size={13} className="coding-chevron" />
        <span className="coding-tool-summary">{call.summary}</span>
        {call.done ? (
          call.isError ? (
            <CircleX size={13} className="coding-tool-state danger" aria-label={t("Failed")} />
          ) : (
            <CircleCheck size={13} className="coding-tool-state ok" aria-label={t("Done")} />
          )
        ) : null}
      </button>
      {open && (
        <div className="coding-tool-body">
          {input && input !== "{}" && <pre>{input}</pre>}
          {call.result !== undefined && (
            <pre className="coding-tool-result">{call.result || t("(no output)")}</pre>
          )}
        </div>
      )}
    </li>
  );
}

function ToolGroup({ calls }: { calls: ToolCall[] }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const failed = calls.filter((c) => c.isError).length;
  const last = calls[calls.length - 1];
  return (
    <div className="coding-tools">
      <button type="button" className="coding-tools-head" aria-expanded={open} onClick={() => setOpen(!open)}>
        <ChevronRight size={14} className="coding-chevron" />
        <Wrench size={13} />
        <span>
          {calls.length} {t(calls.length === 1 ? "tool call" : "tool calls")}
        </span>
        {failed > 0 && (
          <span className="xc-badge danger">
            {failed} {t("failed")}
          </span>
        )}
        {!open && <span className="coding-tools-last">{last.summary}</span>}
      </button>
      {open && (
        <ul className="coding-tool-list">
          {calls.map((c) => (
            <ToolRow key={c.id} call={c} />
          ))}
        </ul>
      )}
    </div>
  );
}

interface Props {
  events: TaskEvent[];
  running: boolean;
}

/** 执行器的实时输出。离底部近时自动滚到最新。 */
export default function OutputView({ events, running }: Props) {
  const t = useT();
  const blocks = useMemo(() => buildBlocks(events), [events]);
  const box = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  useEffect(() => {
    const el = box.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [blocks]);
  return (
    <div
      className="coding-output"
      ref={box}
      onScroll={(e) => {
        const el = e.currentTarget;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
      }}
      role="log"
      aria-live="polite"
      aria-label={t("Output")}
    >
      {blocks.length === 0 && !running && <p className="xc-muted">{t("No output yet.")}</p>}
      {blocks.map((b) => {
        switch (b.type) {
          case "text":
            return (
              <p key={b.seq} className={`coding-text${b.stderr ? " stderr" : ""}`}>
                {b.text}
              </p>
            );
          case "status":
            return (
              <p key={b.seq} className="coding-line status">
                <Info size={13} /> {statusText(b, t)}
              </p>
            );
          case "error":
            return (
              <p key={b.seq} className="coding-line error">
                <AlertTriangle size={13} /> {b.text}
              </p>
            );
          case "done":
            return (
              <p key={b.seq} className={`coding-line done${b.reason === "exited" && b.exitCode === 0 ? " ok" : ""}`}>
                {doneText(b, t)}
              </p>
            );
          case "tools":
            return <ToolGroup key={b.seq} calls={b.calls} />;
        }
        return null;
      })}
      {running && (
        <p className="coding-line status">
          <Loader2 size={13} className="coding-spin" /> {t("Working...")}
        </p>
      )}
    </div>
  );
}
