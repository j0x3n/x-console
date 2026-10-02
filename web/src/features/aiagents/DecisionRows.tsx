import { useState } from "react";
import { Link } from "react-router";
import { MessageCircleQuestion, Send, ShieldQuestion } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { issuePath } from "../projects/logic";
import { useAssistant } from "../assistant/store";
import { useAnswerDecision, type AiAgentDecision } from "./api";
import "./i18n";
import "./aiagents.css";

/** 点标题去哪里：卡片、Agent 详情，或者 Agent 列表。 */
function decisionLink(d: AiAgentDecision): string {
  if (d.issueKey) return issuePath(d.issueKey);
  if (d.agentId) return `/coding/agents/${d.agentId}`;
  return "/coding";
}

/**
 * B87：今日页“待你决定”里 Agent 的权限请求和问题。
 * 权限请求直接批准或拒绝；问题点选项，或者自己写回答。
 */
export function DecisionRow({ decision: d }: { decision: AiAgentDecision }) {
  const t = useT();
  const language = useLanguage();
  const answer = useAnswerDecision();
  const [writing, setWriting] = useState(false);
  const [text, setText] = useState("");
  const send = (body: { approve?: boolean; answer?: string }, done: string) =>
    answer.mutate(
      { id: d.id, ...body },
      {
        onSuccess: () => toast(t(done)),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  const question = d.kind === "question";
  const Icon = question ? MessageCircleQuestion : ShieldQuestion;
  const meta = [d.agentName, d.issueKey, relativeTime(d.createdAt, language)]
    .filter(Boolean)
    .join(" · ");
  return (
    <div className="today-row aiagent-decision">
      <span className="today-row-icon accent">
        <Icon size={15} />
      </span>
      <Link
        className="today-row-main"
        to={decisionLink(d)}
        title={d.detail}
        onClick={(event) => {
          if (d.conversationId) {
            event.preventDefault();
            useAssistant.getState().setConversation(d.conversationId);
            useAssistant.getState().setOpen(true);
          }
        }}
      >
        <strong>{d.title}</strong>
        <small>
          {question ? t("Agent question") : t("Permission request")}
          {meta && ` · ${meta}`}
        </small>
      </Link>
      <div className="aiagent-decision-actions">
        {question ? (
          <>
            {(d.options ?? []).slice(0, 3).map((o) => (
              <button
                key={o}
                className="xc-btn small"
                disabled={answer.isPending}
                onClick={() => send({ answer: o }, "Answered")}
              >
                {o}
              </button>
            ))}
            <button
              className="xc-btn small"
              aria-expanded={writing}
              onClick={() => setWriting((v) => !v)}
            >
              {t("Answer")}
            </button>
          </>
        ) : (
          <>
            <button
              className="xc-btn small"
              disabled={answer.isPending}
              onClick={() => send({ approve: false }, "Rejected")}
            >
              {t("Reject")}
            </button>
            <button
              className="xc-btn small primary"
              disabled={answer.isPending}
              onClick={() => send({ approve: true }, "Approved")}
            >
              {t("Grant")}
            </button>
          </>
        )}
      </div>
      {writing && (
        <form
          className="aiagent-decision-answer"
          onSubmit={(e) => {
            e.preventDefault();
            if (!text.trim()) return;
            send({ answer: text.trim() }, "Answered");
            setWriting(false);
            setText("");
          }}
        >
          <input
            className="xc-input"
            autoFocus
            value={text}
            placeholder={t("Your answer")}
            aria-label={t("Your answer")}
            onChange={(e) => setText(e.target.value)}
          />
          <button
            className="xc-btn small primary"
            disabled={!text.trim() || answer.isPending}
            aria-label={t("Send")}
            title={t("Send")}
          >
            <Send size={13} />
          </button>
        </form>
      )}
    </div>
  );
}
