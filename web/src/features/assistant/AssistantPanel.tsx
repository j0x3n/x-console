import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import { Link, useLocation } from "react-router";
import {
  ArrowUp,
  Bot,
  ChevronDown,
  Maximize2,
  Minimize2,
  Plus,
  Sparkles,
  Square,
  Trash2,
  X,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "../../api/client";
import { onServerEvent } from "../../api/events";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import {
  aiKeys,
  isNotLive,
  useAiSettings,
  useConversation,
  useConversations,
  useDeleteConversation,
  useSendMessage,
  useStopReply,
  useTools,
  type PageContext,
} from "./api";
import Timeline from "./components/Timeline";
import { buildTimeline, conversationTitle } from "./logic";
import { clampOffset, useAssistant } from "./store";
import "./i18n";
import "./assistant.css";
import { confirmAction } from "../../components/ui/ConfirmDialog";

const PANEL = { width: 400, height: 600 };

/*
 * AI（B3，原名 AI 助手）：右下角的圆形按钮，点开是浮窗。⌘J 开关。
 * 浮窗可以拖动，也可以放大成右侧整列。手机上是全屏的底部弹层。
 */
export default function AssistantPanel() {
  const t = useT();
  const open = useAssistant((s) => s.open);
  const toggle = useAssistant((s) => s.toggle);
  useAssistantEvents();

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "j") {
        e.preventDefault();
        toggle();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [toggle]);

  return (
    <>
      {!open && (
        <button
          type="button"
          className="ai-fab"
          aria-label={`${t("AI")} (⌘J)`}
          title={`${t("AI")} (⌘J)`}
          onClick={toggle}
        >
          <Sparkles size={20} />
        </button>
      )}
      {open && <Panel />}
    </>
  );
}

/** 处理 ai.* 事件：文字增量、存了新消息、有动作待确认、一轮结束、出错。 */
function useAssistantEvents() {
  const qc = useQueryClient();
  useEffect(
    () =>
      onServerEvent((event) => {
        if (!event.topic.startsWith("ai.")) return;
        const data = (event.data ?? {}) as {
          conversationId?: number;
          text?: string;
          message?: string;
        };
        const id = data.conversationId;
        if (!id) return;
        const s = useAssistant.getState();
        const refresh = () =>
          qc.invalidateQueries({ queryKey: aiKeys.conversation(id) });
        switch (event.topic) {
          case "ai.delta":
            s.appendDelta(id, data.text ?? "");
            break;
          case "ai.message_saved":
          case "ai.action_pending":
            // 存下来的消息已经包含这段文字，清掉临时的。
            s.clearStreaming(id);
            refresh();
            break;
          case "ai.message_done":
            s.clearStreaming(id);
            refresh();
            qc.invalidateQueries({ queryKey: aiKeys.conversations });
            break;
          case "ai.error":
            s.setError(id, data.message ?? "出错了");
            refresh();
            break;
        }
      }),
    [qc],
  );
}

function pageContext(pathname: string): PageContext {
  const h1 = document.querySelector("#main .xc-page-title h1, #main h1");
  return { path: pathname, title: h1?.textContent?.trim() ?? "" };
}

function Panel() {
  const t = useT();
  const location = useLocation();
  const setOpen = useAssistant((s) => s.setOpen);
  const expanded = useAssistant((s) => s.expanded);
  const setExpanded = useAssistant((s) => s.setExpanded);
  const offset = useAssistant((s) => s.offset);
  const setOffset = useAssistant((s) => s.setOffset);
  const conversationId = useAssistant((s) => s.conversationId);
  const setConversation = useAssistant((s) => s.setConversation);
  const streaming = useAssistant((s) =>
    conversationId ? s.streaming[conversationId] : undefined,
  );
  const liveError = useAssistant((s) =>
    conversationId ? s.errors[conversationId] : undefined,
  );

  const conversations = useConversations();
  const notLive = conversations.isError && isNotLive(conversations.error);
  const settings = useAiSettings(!notLive);
  const tools = useTools(!notLive);
  const detail = useConversation(notLive ? null : conversationId);
  const send = useSendMessage();
  const stop = useStopReply();
  const remove = useDeleteConversation();

  const [text, setText] = useState("");
  const [historyOpen, setHistoryOpen] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // 当前对话被删了或不存在时，回到新对话。
  useEffect(() => {
    if (detail.isError) setConversation(null);
  }, [detail.isError]);

  const running = !!detail.data?.running || send.isPending;
  const items = useMemo(
    () =>
      detail.data
        ? buildTimeline(
            detail.data.messages,
            detail.data.pendingActions,
            tools.data ?? [],
            { running, streaming },
          )
        : [],
    [detail.data, tools.data, running, streaming],
  );

  // 有新内容时滚到底。
  useEffect(() => {
    const el = bodyRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items.length, streaming]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [conversationId]);

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    const value = text.trim();
    if (!value || running) return;
    send.mutate(
      {
        conversationId,
        text: value,
        context: pageContext(location.pathname),
      },
      {
        onSuccess: (id) => {
          setText("");
          if (id !== conversationId) setConversation(id);
        },
      },
    );
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // 输入法选字时的回车不发送。
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  // 拖动标题栏移动浮窗。放大和手机上不能拖。
  const drag = useRef<{ x: number; y: number; ox: number; oy: number } | null>(
    null,
  );
  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (expanded || window.innerWidth <= 640) return;
    if ((e.target as HTMLElement).closest("button")) return;
    drag.current = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPointerMove = useCallback(
    (e: PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d) return;
      setOffset(
        clampOffset(
          { x: d.ox - (e.clientX - d.x), y: d.oy - (e.clientY - d.y) },
          PANEL,
          { width: window.innerWidth, height: window.innerHeight },
        ),
      );
    },
    [setOffset],
  );
  const onPointerUp = () => {
    drag.current = null;
  };

  const title = detail.data
    ? conversationTitle(detail.data.conversation.title)
    : t("New conversation");
  const style = expanded
    ? undefined
    : { right: 20 + offset.x, bottom: 20 + offset.y };

  return (
    <section
      className={`ai-panel${expanded ? " expanded" : ""}`}
      style={style}
      role="dialog"
      aria-label={t("AI")}
    >
      <header
        className="ai-head"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
      >
        <Bot size={16} className="ai-head-icon" />
        <div className="ai-history-wrap">
          <button
            type="button"
            className="ai-title"
            aria-expanded={historyOpen}
            disabled={notLive}
            onClick={() => setHistoryOpen((v) => !v)}
          >
            <span>{title}</span>
            <ChevronDown size={13} />
          </button>
          {historyOpen && conversations.data && (
            <>
              <div
                className="ai-menu-backdrop"
                onClick={() => setHistoryOpen(false)}
              />
              <div className="ai-history" role="menu">
                {conversations.data.length === 0 && (
                  <p className="ai-muted">{t("No conversations yet")}</p>
                )}
                {conversations.data.map((c) => (
                  <div
                    key={c.id}
                    className={`ai-history-item${c.id === conversationId ? " active" : ""}`}
                  >
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setConversation(c.id);
                        setHistoryOpen(false);
                      }}
                    >
                      {conversationTitle(c.title)}
                    </button>
                    <button
                      type="button"
                      className="ai-icon-btn"
                      aria-label={`${t("Delete")} ${conversationTitle(c.title)}`}
                      onClick={async () =>
                        (await confirmAction({
                          title: `${t("Delete")}“${conversationTitle(c.title)}”？`,
                        })) &&
                        remove.mutate(c.id, {
                          onSuccess: () => {
                            if (c.id === conversationId) setConversation(null);
                          },
                        })
                      }
                    >
                      <Trash2 size={13} />
                    </button>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
        <span className="xc-spacer" />
        <button
          type="button"
          className="ai-icon-btn"
          title={t("New conversation")}
          aria-label={t("New conversation")}
          disabled={notLive}
          onClick={() => {
            setConversation(null);
            setText("");
          }}
        >
          <Plus size={16} />
        </button>
        <button
          type="button"
          className="ai-icon-btn ai-expand"
          title={expanded ? t("Shrink") : t("Expand")}
          aria-label={expanded ? t("Shrink") : t("Expand")}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
        <button
          type="button"
          className="ai-icon-btn"
          title={t("Close")}
          aria-label={t("Close")}
          onClick={() => setOpen(false)}
        >
          <X size={16} />
        </button>
      </header>

      <div className="ai-body" ref={bodyRef} aria-live="polite">
        {notLive ? (
          <div className="ai-empty">
            <Sparkles size={26} />
            <strong>AI 还没上线</strong>
            <p>界面已经做好，服务端还在开发。</p>
          </div>
        ) : settings.data && !settings.data.hasApiKey ? (
          <div className="ai-empty">
            <Sparkles size={26} />
            <strong>还没有填 API Key</strong>
            <p>
              去 <Link to="/settings/assistant">设置 → AI</Link> 填上 Anthropic
              的 API Key 就能用了。
            </p>
          </div>
        ) : conversationId && detail.isPending ? (
          <Loading />
        ) : items.length === 0 && !running ? (
          <div className="ai-empty">
            <Sparkles size={26} />
            <strong>有什么要我做的？</strong>
            <p>
              我能查看和修改笔记、提醒、项目、习惯、云盘，也能看服务器状态。
            </p>
            <p className="ai-muted">隐藏内容我看不到。</p>
          </div>
        ) : (
          <>
            <Timeline items={items} />
            {running &&
              !streaming &&
              !items.some(
                (i) => i.kind === "action" && i.status === "waiting",
              ) && (
                <div className="ai-thinking" role="status">
                  <span />
                  <span />
                  <span />
                </div>
              )}
          </>
        )}
        {(liveError || send.isError) && (
          <p className="xc-error-text ai-error">
            {liveError ?? errorMessage(send.error)}
          </p>
        )}
      </div>

      <form className="ai-composer" onSubmit={submit}>
        <textarea
          ref={inputRef}
          rows={1}
          value={text}
          disabled={notLive}
          placeholder={t("Ask or tell me what to do")}
          aria-label={t("Message")}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
        />
        {running && conversationId ? (
          <button
            type="button"
            className="ai-send stop"
            title={t("Stop")}
            aria-label={t("Stop")}
            onClick={() => stop.mutate(conversationId)}
          >
            <Square size={13} />
          </button>
        ) : (
          <button
            className="ai-send"
            title={t("Send")}
            aria-label={t("Send")}
            disabled={!text.trim() || notLive}
          >
            <ArrowUp size={16} />
          </button>
        )}
      </form>
    </section>
  );
}
