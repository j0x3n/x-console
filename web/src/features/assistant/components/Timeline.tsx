import { useState } from "react";
import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronRight,
  Loader2,
  ShieldAlert,
  X,
} from "lucide-react";
import { errorMessage } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import { useT } from "../../../contexts/LanguageContext";
import { useDecideAction } from "../api";
import { summarizeInput, type TimelineItem } from "../logic";

export default function Timeline({ items }: { items: TimelineItem[] }) {
  return (
    <>
      {items.map((item) =>
        item.kind === "user" ? (
          <div key={item.key} className="ai-msg user">
            <div className="ai-bubble">{item.text}</div>
          </div>
        ) : item.kind === "assistant" ? (
          <div key={item.key} className="ai-msg assistant">
            <Markdown source={item.text} className="ai-md" />
            {item.streaming && <span className="ai-caret" aria-hidden />}
          </div>
        ) : (
          <ActionCard key={item.key} item={item} />
        ),
      )}
    </>
  );
}

type ActionItem = Extract<TimelineItem, { kind: "action" }>;

const STATUS_LABELS: Record<ActionItem["status"], string> = {
  running: "Running",
  waiting: "Waiting for you",
  ok: "Done",
  error: "Failed",
  rejected: "Rejected",
};

/*
 * 一次动作一行小卡片：名称、参数摘要、状态。点开看完整参数和结果。
 * 等确认的动作直接展开，下面是确认和拒绝按钮。
 */
function ActionCard({ item }: { item: ActionItem }) {
  const t = useT();
  const decide = useDecideAction();
  const [open, setOpen] = useState(false);
  const waiting = item.status === "waiting" && item.pending;
  const expanded = open || !!waiting;
  return (
    <div className={`ai-action ${item.status}${waiting ? " ask" : ""}`}>
      <button
        type="button"
        className="ai-action-head"
        aria-expanded={expanded}
        onClick={() => setOpen((v) => !v)}
      >
        <StatusIcon status={item.status} />
        <strong>{item.title}</strong>
        <span className="ai-action-sum">{summarizeInput(item.input)}</span>
        {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
      </button>
      {expanded && (
        <div className="ai-action-body">
          {waiting && (
            <p className="ai-action-ask">
              {item.effect === "dangerous" ? (
                <>
                  <ShieldAlert size={14} />
                  这是高危操作，确认后要再验证一次。
                </>
              ) : (
                "助手要执行这个操作，确认吗？"
              )}
            </p>
          )}
          <dl className="ai-params">
            {Object.entries(item.input).map(([k, v]) => (
              <div key={k}>
                <dt>{k}</dt>
                <dd>{typeof v === "string" ? v : JSON.stringify(v)}</dd>
              </div>
            ))}
            {Object.keys(item.input).length === 0 && (
              <div>
                <dd className="ai-muted">{t("No parameters")}</dd>
              </div>
            )}
          </dl>
          {item.result != null && item.result !== "" && (
            <pre className="ai-result">
              {typeof item.result === "string"
                ? item.result
                : JSON.stringify(item.result, null, 2)}
            </pre>
          )}
          {waiting && (
            <>
              {decide.isError && (
                <p className="xc-error-text">{errorMessage(decide.error)}</p>
              )}
              <div className="ai-action-buttons">
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={decide.isPending}
                  onClick={() =>
                    decide.mutate({ action: item.pending!, approve: false })
                  }
                >
                  <X size={13} /> {t("Reject")}
                </button>
                <button
                  type="button"
                  className={`xc-btn small ${item.effect === "dangerous" ? "danger" : "primary"}`}
                  disabled={decide.isPending}
                  onClick={() =>
                    decide.mutate({ action: item.pending!, approve: true })
                  }
                >
                  <Check size={13} /> {t("Approve")}
                </button>
              </div>
            </>
          )}
        </div>
      )}
      <span className="ai-sr">{t(STATUS_LABELS[item.status])}</span>
    </div>
  );
}

function StatusIcon({ status }: { status: ActionItem["status"] }) {
  const t = useT();
  const label = t(STATUS_LABELS[status]);
  if (status === "running")
    return <Loader2 size={13} className="ai-spin" aria-label={label} />;
  if (status === "ok")
    return <Check size={13} className="ok" aria-label={label} />;
  if (status === "waiting")
    return <ShieldAlert size={13} className="warn" aria-label={label} />;
  if (status === "rejected")
    return <X size={13} className="muted" aria-label={label} />;
  return <AlertTriangle size={13} className="danger" aria-label={label} />;
}
