import { useState } from "react";
import { Link } from "react-router";
import {
  AlertTriangle,
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  FileText,
  Loader2,
  ShieldAlert,
  X,
} from "lucide-react";
import { errorMessage } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import { useT } from "../../../contexts/LanguageContext";
import { attachmentUrl, useDecideAction } from "../api";
import {
  foldLines,
  operateSummary,
  rememberedText,
  summarizeInput,
  type TimelineItem,
} from "../logic";

export default function Timeline({ items }: { items: TimelineItem[] }) {
  return (
    <>
      {items.map((item) =>
        item.kind === "user" ? (
          <div key={item.key} className="ai-msg user">
            {item.attachments && (
              <div className="ai-attachments">
                {item.attachments.map((a) =>
                  a.kind === "image" ? (
                    <a
                      key={a.id}
                      href={attachmentUrl(a.id)}
                      target="_blank"
                      rel="noreferrer"
                      title={a.name}
                    >
                      <img
                        className="ai-attach-img"
                        src={attachmentUrl(a.id)}
                        alt={a.name}
                      />
                    </a>
                  ) : (
                    <span key={a.id} className="ai-attach-chip" title={a.name}>
                      <FileText size={13} />
                      <span>{a.name}</span>
                    </span>
                  ),
                )}
              </div>
            )}
            {item.text && <div className="ai-bubble">{item.text}</div>}
          </div>
        ) : item.kind === "assistant" ? (
          <div key={item.key} className="ai-msg assistant">
            <Markdown source={item.text} className="ai-md" />
            {item.streaming && <span className="ai-caret" aria-hidden />}
          </div>
        ) : (
          <div key={item.key}>
            <ActionCard item={item} />
            <Remembered item={item} />
            <OperateCard item={item} />
          </div>
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
  const [full, setFull] = useState(false);
  const waiting = item.status === "waiting" && item.pending;
  const expanded = open || !!waiting;
  // 机器 Agent 的命令单独显示，其他参数照常列出来。
  const command =
    typeof item.input.command === "string" ? item.input.command : null;
  const reason =
    typeof item.input.reason === "string" ? item.input.reason : null;
  const params = Object.entries(item.input).filter(
    ([k]) => !(command && (k === "command" || k === "reason")),
  );
  const resultText =
    item.result == null || item.result === ""
      ? ""
      : typeof item.result === "string"
        ? item.result
        : JSON.stringify(item.result, null, 2);
  const folded = full ? { text: resultText, hidden: 0 } : foldLines(resultText);
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
        <span className={`ai-action-sum${command ? " xc-mono" : ""}`}>
          {command ?? summarizeInput(item.input)}
        </span>
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
                "要执行这个操作，确认吗？"
              )}
            </p>
          )}
          {reason && <p className="ai-action-reason">{reason}</p>}
          {command && <pre className="ai-command">$ {command}</pre>}
          <dl className="ai-params">
            {params.map(([k, v]) => (
              <div key={k}>
                <dt>{k}</dt>
                <dd>{typeof v === "string" ? v : JSON.stringify(v)}</dd>
              </div>
            ))}
            {params.length === 0 && !command && (
              <div>
                <dd className="ai-muted">{t("No parameters")}</dd>
              </div>
            )}
          </dl>
          {resultText && <pre className="ai-result">{folded.text}</pre>}
          {folded.hidden > 0 && (
            <button
              type="button"
              className="xc-btn ghost small"
              onClick={() => setFull(true)}
            >
              {t("Show all")}
            </button>
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

/** “已记住：……”，点了去设置 → AI 的记忆卡片（B61）。 */
function Remembered({ item }: { item: TimelineItem }) {
  const t = useT();
  const text = rememberedText(item);
  if (!text) return null;
  return (
    <Link className="ai-remembered" to="/settings/assistant#ai-memory">
      {t("Remembered")}：{text}
    </Link>
  );
}

/** Agent 代为操作机器的结果：Agent、机器、几条命令、去看过程（B60）。 */
function OperateCard({ item }: { item: TimelineItem }) {
  const t = useT();
  const s = operateSummary(item);
  if (!s) return null;
  return (
    <div className="ai-operate">
      <Bot size={14} />
      <span>
        <strong>{s.agent}</strong> · {s.host} ·{" "}
        {t("ran N commands").replace("N", String(s.commands))}
        {s.waiting > 0 && (
          <> · {t("N waiting for you").replace("N", String(s.waiting))}</>
        )}
      </span>
      <Link to={s.link}>{t("View the steps")}</Link>
    </div>
  );
}
