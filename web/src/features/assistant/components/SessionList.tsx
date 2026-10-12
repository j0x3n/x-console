import { useState } from "react";
import { Loader2, Search, Server, Sparkles } from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import AgentAvatar from "../../aiagents/AgentAvatar";
import type { AiSession } from "../api";
import { conversationTitle } from "../logic";
import {
  filterSessions,
  groupSessions,
  isSelected,
  SESSION_FILTERS,
  STATUS_LABELS,
  statusTone,
  type SessionFilter,
} from "../sessions";

/** 会话的图标：面板对话是 AI 图标，服务器是服务器图标，Agent 用它自己的头像。 */
export function SessionIcon({
  session,
  size = 28,
}: {
  session: Pick<
    AiSession,
    "kind" | "sourceName" | "agentAvatar" | "agentColor" | "status"
  >;
  size?: number;
}) {
  if (session.kind === "agent")
    return (
      <AgentAvatar
        agent={{
          name: session.sourceName,
          avatar: session.agentAvatar ?? "",
          color: session.agentColor ?? "",
        }}
        size={size}
        failed={session.status === "failed"}
      />
    );
  const Icon = session.kind === "host" ? Server : Sparkles;
  return (
    <span
      className={`ai-session-icon ${session.kind}`}
      style={{ width: size, height: size }}
      aria-hidden
    >
      <Icon size={Math.round(size * 0.5)} />
    </span>
  );
}

/** 来源标签：对话 / 服务器 · hk-1 / Agent · 小码。 */
export function sourceLabel(
  s: Pick<AiSession, "kind" | "sourceName">,
  t: (key: string) => string,
) {
  const kind = t(
    s.kind === "host" ? "Server" : s.kind === "agent" ? "AI agent" : "Chat",
  );
  return s.sourceName ? `${kind} · ${s.sourceName}` : kind;
}

export function StatusBadge({ session }: { session: AiSession }) {
  const t = useT();
  if (session.status === "idle") return null;
  const label =
    session.status === "waiting" && session.pendingCount > 1
      ? `${t(STATUS_LABELS.waiting)} ${session.pendingCount}`
      : t(STATUS_LABELS[session.status]);
  return (
    <span className={`ai-status ${statusTone(session.status)}`}>
      {session.status === "working" ? (
        <Loader2 size={11} className="ai-spin" />
      ) : (
        <i aria-hidden />
      )}
      {label}
    </span>
  );
}

/**
 * B183：AI 窗口左边的会话列表。顶部是筛选和搜索，下面“进行中”和“最近”两组。
 */
export default function SessionList({
  sessions,
  conversationId,
  runId,
  onPick,
}: {
  sessions: AiSession[];
  conversationId: number | null;
  runId: number | null;
  onPick: (s: AiSession) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const [filter, setFilter] = useState<SessionFilter>("all");
  const [query, setQuery] = useState("");
  const shown = filterSessions(sessions, filter, query);
  const groups = groupSessions(shown);
  const activeCount = groupSessions(sessions).active.length;

  const row = (s: AiSession) => {
    const on = isSelected(s, conversationId, runId);
    const title = conversationTitle(s.title);
    return (
      <li key={s.id}>
        <button
          type="button"
          className={`ai-session${on ? " active" : ""}`}
          aria-current={on || undefined}
          onClick={() => onPick(s)}
        >
          <SessionIcon session={s} />
          <span className="ai-session-main">
            <span className="ai-session-title" title={title}>
              {title}
            </span>
            <span className="ai-session-meta">
              <span className="ai-session-source">{sourceLabel(s, t)}</span>
              <StatusBadge session={s} />
            </span>
          </span>
          <time className="ai-session-time" dateTime={s.updatedAt}>
            {relativeTime(s.updatedAt, language)}
          </time>
        </button>
      </li>
    );
  };

  return (
    <nav className="ai-sessions" aria-label={t("Conversations")}>
      <div className="ai-sessions-tools">
        <label className="ai-sessions-search">
          <Search size={13} />
          <input
            value={query}
            placeholder={t("Search conversations")}
            aria-label={t("Search conversations")}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <div
          className="ai-filters"
          role="group"
          aria-label={t("Filter conversations")}
        >
          {SESSION_FILTERS.map((f) => (
            <button
              key={f.value}
              type="button"
              className={filter === f.value ? "on" : ""}
              aria-pressed={filter === f.value}
              onClick={() => setFilter(f.value)}
            >
              {t(f.label)}
              {f.value === "active" && activeCount > 0 && (
                <span className="ai-filter-count">{activeCount}</span>
              )}
            </button>
          ))}
        </div>
      </div>
      <div className="ai-sessions-body">
        {shown.length === 0 ? (
          <p className="ai-sessions-empty">
            {sessions.length === 0
              ? t("No conversations yet")
              : t("No matching conversations")}
          </p>
        ) : (
          <>
            {groups.active.length > 0 && (
              <section>
                <h3>{t("In progress")}</h3>
                <ul>{groups.active.map(row)}</ul>
              </section>
            )}
            {groups.recent.length > 0 && (
              <section>
                {groups.active.length > 0 && <h3>{t("Recent")}</h3>}
                <ul>{groups.recent.map(row)}</ul>
              </section>
            )}
          </>
        )}
      </div>
    </nav>
  );
}
