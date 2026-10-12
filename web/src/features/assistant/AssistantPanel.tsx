import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type PointerEvent,
} from "react";
import { Link } from "react-router";
import {
  ChevronLeft,
  ExternalLink,
  Maximize2,
  Minimize2,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { onServerEvent } from "../../api/events";
import { ErrorState } from "../../components/ui/States";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useT } from "../../contexts/LanguageContext";
import {
  aiKeys,
  isNotLive,
  useAiProviders,
  useAiSessions,
  useConversation,
  useConversations,
  useDeleteConversation,
  useModelSettings,
  type AiSession,
} from "./api";
import ConversationView from "./components/ConversationView";
import RunView from "./components/RunView";
import SessionList, {
  SessionIcon,
  sourceLabel,
  StatusBadge,
} from "./components/SessionList";
import { conversationTitle } from "./logic";
import { isSelected, sessionSource } from "./sessions";
import { clampOffset, useAssistant } from "./store";
import "./i18n";
import "./assistant.css";

const PANEL = { width: 760, height: 620 };
/** 窗口比这个窄时，列表和内容分两层显示。 */
const NARROW = 600;

/*
 * AI（B3，B183 重做）：顶栏的 AI 按钮或 ⌘J 打开。左边是全部会话（面板对话、服务器 Agent、
 * Agent 执行记录），右边是选中的那个。浮窗可以拖动，也可以放大成右侧整列。手机上全屏。
 */
export default function AssistantPanel() {
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

  return open ? <Panel /> : null;
}

/** 处理 ai.* 事件：文字增量、存了新消息、有动作待确认、一轮结束、出错。会话列表跟着刷新。 */
function useAssistantEvents() {
  const qc = useQueryClient();
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    // 一轮里事件很多，列表合并成 300 毫秒刷新一次
    const refreshSessions = () => {
      if (timer) return;
      timer = setTimeout(() => {
        timer = undefined;
        qc.invalidateQueries({ queryKey: aiKeys.sessions });
      }, 300);
    };
    const off = onServerEvent((event) => {
      if (event.topic.startsWith("ai_agent.")) {
        refreshSessions();
        return;
      }
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
        case "ai.message_started":
          refresh();
          refreshSessions();
          break;
        case "ai.message_saved":
        case "ai.action_pending":
          // 存下来的消息已经包含这段文字，清掉临时的。
          s.clearStreaming(id);
          refresh();
          refreshSessions();
          break;
        case "ai.message_done":
          s.clearStreaming(id);
          refresh();
          qc.invalidateQueries({ queryKey: aiKeys.conversations });
          qc.invalidateQueries({ queryKey: ["ai", "host-conversations"] });
          refreshSessions();
          break;
        case "ai.permission_changed":
          refresh();
          break;
        case "ai.error":
          s.setError(id, data.message ?? "出错了");
          refresh();
          refreshSessions();
          break;
      }
    });
    return () => {
      off();
      if (timer) clearTimeout(timer);
    };
  }, [qc]);
}

function Panel() {
  const t = useT();
  const qc = useQueryClient();
  const setOpen = useAssistant((s) => s.setOpen);
  const expanded = useAssistant((s) => s.expanded);
  const setExpanded = useAssistant((s) => s.setExpanded);
  const offset = useAssistant((s) => s.offset);
  const setOffset = useAssistant((s) => s.setOffset);
  const conversationId = useAssistant((s) => s.conversationId);
  const runId = useAssistant((s) => s.runId);
  const setConversation = useAssistant((s) => s.setConversation);
  const setRun = useAssistant((s) => s.setRun);
  const listOpen = useAssistant((s) => s.listOpen);
  const setListOpen = useAssistant((s) => s.setListOpen);

  const conversations = useConversations();
  const notLive = conversations.isError && isNotLive(conversations.error);
  const providers = useAiProviders(!notLive);
  const models = useModelSettings(!notLive && !!providers.data);
  const noKey = models.data !== undefined && !models.data.agent;
  const sessions = useAiSessions(!notLive);
  const detail = useConversation(notLive ? null : conversationId);
  const remove = useDeleteConversation();

  // 窄的时候列表和内容分两层：narrowList 为真时只显示列表。
  const ref = useRef<HTMLElement>(null);
  const [narrow, setNarrow] = useState(false);
  const [narrowList, setNarrowList] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const check = () => {
      const width = el.getBoundingClientRect().width;
      // 宽度为 0（还没布局）时不算窄
      setNarrow(width > 0 && width < NARROW);
    };
    check();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const showList = narrow ? narrowList : listOpen;
  const showDetail = !narrow || !narrowList;

  const list = sessions.data ?? [];
  const current: AiSession | undefined = list.find((s) =>
    isSelected(s, conversationId, runId),
  );
  // 选中的执行记录不在列表里了（比如被删掉），回到新对话
  useEffect(() => {
    if (runId && sessions.data && !current) setRun(null);
  }, [runId, sessions.data, current]);

  const pick = (s: AiSession) => {
    if (s.kind === "agent") setRun(s.refId);
    else setConversation(s.refId);
    setNarrowList(false);
  };
  const startNew = () => {
    setConversation(null);
    setNarrowList(false);
  };

  // 拖动标题栏移动浮窗。放大和手机上不能拖。
  const drag = useRef<{ x: number; y: number; ox: number; oy: number } | null>(
    null,
  );
  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (expanded || window.innerWidth <= 640) return;
    if (
      e.button !== 0 ||
      (e.target as HTMLElement).closest("button, input, select, a")
    )
      return;
    // 不让浏览器开始选中文字或拖拽，否则收不到松开事件，浮窗会一直跟着鼠标
    e.preventDefault();
    drag.current = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPointerMove = useCallback(
    (e: PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d) return;
      // 松开事件丢了（比如在窗口外松开）时，按键已经不在按着
      if (e.buttons === 0) {
        drag.current = null;
        return;
      }
      const rect = ref.current?.getBoundingClientRect();
      setOffset(
        clampOffset(
          { x: d.ox - (e.clientX - d.x), y: d.oy - (e.clientY - d.y) },
          rect ? { width: rect.width, height: rect.height } : PANEL,
          { width: window.innerWidth, height: window.innerHeight },
        ),
      );
    },
    [setOffset],
  );
  const onPointerUp = (e: PointerEvent<HTMLElement>) => {
    drag.current = null;
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture?.(e.pointerId))
      el.releasePointerCapture(e.pointerId);
  };

  const conv = detail.data?.conversation;
  const title = runId
    ? (current?.title ?? "")
    : conv
      ? conversationTitle(conv.title)
      : t("New conversation");
  const source = current ? sessionSource(current) : null;
  const deletable = !runId && !!conversationId && !!conv;
  const style = expanded
    ? undefined
    : { right: 20 + offset.x, bottom: 20 + offset.y };

  return (
    <section
      ref={ref}
      className={`ai-panel${expanded ? " expanded" : ""}${showList ? " with-list" : ""}${narrow ? " narrow" : ""}`}
      style={style}
      role="dialog"
      aria-label={t("AI")}
    >
      <header
        className="ai-head"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onLostPointerCapture={() => (drag.current = null)}
      >
        <button
          type="button"
          className="ai-icon-btn"
          title={showList ? t("Hide list") : t("Show list")}
          aria-label={showList ? t("Hide list") : t("Show list")}
          aria-expanded={showList}
          disabled={notLive}
          onClick={() =>
            narrow ? setNarrowList(!narrowList) : setListOpen(!listOpen)
          }
        >
          {showList ? (
            <PanelLeftClose size={16} />
          ) : (
            <PanelLeftOpen size={16} />
          )}
        </button>
        <span className="ai-brand">
          <Sparkles size={15} />
          AI
        </span>
        <span className="xc-spacer" />
        <button
          type="button"
          className="ai-icon-btn"
          title={t("New conversation")}
          aria-label={t("New conversation")}
          disabled={notLive}
          onClick={startNew}
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

      {notLive ? (
        <div className="ai-body">
          <div className="ai-empty">
            <Sparkles size={26} />
            <strong>AI 还没上线</strong>
            <p>界面已经做好，服务端还在开发。</p>
          </div>
        </div>
      ) : providers.isError ? (
        <div className="ai-body">
          <ErrorState
            error={providers.error}
            onRetry={() => providers.refetch()}
          />
        </div>
      ) : (
        <div className="ai-main">
          {showList && (
            <SessionList
              sessions={list}
              conversationId={conversationId}
              runId={runId}
              onPick={pick}
            />
          )}
          {showDetail && (
            <div className="ai-detail">
              <div className="ai-detail-head">
                {narrow && (
                  <button
                    type="button"
                    className="ai-icon-btn"
                    title={t("Back")}
                    aria-label={t("Back")}
                    onClick={() => setNarrowList(true)}
                  >
                    <ChevronLeft size={16} />
                  </button>
                )}
                {current ? (
                  <SessionIcon session={current} size={26} />
                ) : (
                  <SessionIcon
                    session={{ kind: "chat", sourceName: "", status: "idle" }}
                    size={26}
                  />
                )}
                <div className="ai-detail-title">
                  <strong title={title}>{title}</strong>
                  {current && (
                    <span className="ai-detail-sub">
                      <span>{sourceLabel(current, t)}</span>
                      <StatusBadge session={current} />
                    </span>
                  )}
                </div>
                {source && (
                  <Link
                    className="xc-btn small ghost ai-source"
                    to={source}
                    title={t("Open where it runs")}
                  >
                    <ExternalLink size={13} />
                    <span>{t("Open where it runs")}</span>
                  </Link>
                )}
                {deletable && (
                  <button
                    type="button"
                    className="ai-icon-btn"
                    title={t("Delete")}
                    aria-label={`${t("Delete")} ${title}`}
                    onClick={async () =>
                      (await confirmAction({
                        title: `${t("Delete")}“${title}”？`,
                      })) &&
                      remove.mutate(conversationId, {
                        onSuccess: () => {
                          setConversation(null);
                          qc.invalidateQueries({ queryKey: aiKeys.sessions });
                        },
                      })
                    }
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
              {runId ? (
                <RunView key={runId} runId={runId} session={current} />
              ) : (
                <ConversationView
                  key={conversationId ?? "new"}
                  conversationId={conversationId}
                  noKey={noKey}
                />
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
