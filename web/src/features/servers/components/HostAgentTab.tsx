import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { Link, useSearchParams } from "react-router";
import { ArrowUp, Bot, Plus, Square, Trash2 } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage, unwrap } from "../../../api/client";
import { onServerEvent } from "../../../api/events";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { ErrorState, Loading, NotLive } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  aiApi,
  aiKeys,
  isNotLive,
  useConversation,
  useCreateHostConversation,
  useDeleteConversation,
  useHostConversations,
  useModelSettings,
  useSetPermission,
  useStopReply,
  type HostAgentPermission,
} from "../../assistant/api";
import Timeline from "../../assistant/components/Timeline";
import {
  buildTimeline,
  conversationTitle,
  HOST_TOOLS,
} from "../../assistant/logic";
import { useAssistant } from "../../assistant/store";
import type { HostDetail } from "../api";
import "../../assistant/assistant.css";
import "../../assistant/i18n";

const PERMISSIONS: { value: HostAgentPermission; label: string }[] = [
  { value: "confirm", label: "Confirm each step" },
  { value: "read_auto", label: "Run read-only commands" },
  { value: "all_auto", label: "Run everything" },
];

const EXAMPLES = [
  "Why is the disk almost full?",
  "Any errors in the logs in the last hour?",
  "Why won't nginx start?",
];

/*
 * 机器详情的 Agent 标签（B33）。AI 跑在面板上，通过代理在这台机器上执行操作。
 * 宽屏左边是这台机器的会话，窄屏换成顶部的下拉框。
 */
export default function HostAgentTab({ host }: { host: HostDetail }) {
  const t = useT();
  const list = useHostConversations(host.id);
  if (list.isPending) return <Loading />;
  if (list.isError)
    return isNotLive(list.error) ? (
      <NotLive name={t("AI agent")} icon={<Bot size={22} />} />
    ) : (
      <ErrorState error={list.error} onRetry={() => list.refetch()} />
    );
  return <AgentView host={host} conversations={list.data} />;
}

function AgentView({
  host,
  conversations,
}: {
  host: HostDetail;
  conversations: {
    id: number;
    title: string;
    updatedAt: string;
  }[];
}) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  // ?c=<会话 id>：面板 AI 交给 Agent 操作这台机器后，从回复里点过来（B60）
  const [params] = useSearchParams();
  const wanted = Number(params.get("c"));
  const [selected, setSelected] = useState<number | null>(
    conversations.some((c) => c.id === wanted)
      ? wanted
      : (conversations[0]?.id ?? null),
  );
  const [draftMode, setDraftMode] = useState<HostAgentPermission>("confirm");
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const bodyRef = useRef<HTMLDivElement>(null);

  const models = useModelSettings();
  const detail = useConversation(selected);
  const create = useCreateHostConversation(host.id);
  const remove = useDeleteConversation();
  const stop = useStopReply();
  const setPermission = useSetPermission();
  const streaming = useAssistant((s) =>
    selected ? s.streaming[selected] : undefined,
  );
  const liveError = useAssistant((s) =>
    selected ? s.errors[selected] : undefined,
  );

  // 选中的会话不存在了（在别处删掉了），回到新会话。
  useEffect(() => {
    if (detail.isError) setSelected(null);
  }, [detail.isError]);

  // 一轮结束后刷新这台机器的会话列表（标题和时间会变）。
  useEffect(
    () =>
      onServerEvent((event) => {
        if (event.topic === "ai.message_done")
          qc.invalidateQueries({
            queryKey: aiKeys.hostConversations(host.id),
          });
      }),
    [qc, host.id],
  );

  const mode: HostAgentPermission = selected
    ? (detail.data?.conversation.permission ?? "confirm")
    : draftMode;

  // “全部自动”只在这次打开页面时有效：离开会话或离开页面就改回每步确认。
  // autoIds 记下这次打开页面时改成“全部自动”的会话。
  const autoIds = useRef(new Set<number>());
  const resetRef = useRef(setPermission.mutate);
  resetRef.current = setPermission.mutate;
  useEffect(
    () => () => {
      if (selected && autoIds.current.delete(selected))
        resetRef.current({ id: selected, mode: "confirm" });
    },
    [selected],
  );

  const running = !!detail.data?.running || sending;
  const tools = useMemo(
    () => HOST_TOOLS.map((tool) => ({ ...tool, title: t(tool.title) })),
    [t],
  );
  const items = useMemo(
    () =>
      detail.data
        ? buildTimeline(
            detail.data.messages,
            detail.data.pendingActions,
            tools,
            { running, streaming },
          )
        : [],
    [detail.data, tools, running, streaming],
  );

  useEffect(() => {
    const el = bodyRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items.length, streaming]);

  const noModel = models.data !== undefined && !models.data.agent;

  const changeMode = async (next: HostAgentPermission) => {
    if (next === mode) return;
    if (
      next === "all_auto" &&
      !(await confirmAction({
        title: t("Run everything without asking?"),
        description: t(
          "Dangerous commands still need your confirmation. This only lasts while this page is open.",
        ),
        confirmLabel: t("Run everything"),
      }))
    )
      return;
    if (!selected) {
      setDraftMode(next);
      return;
    }
    const id = selected;
    setPermission.mutate(
      { id, mode: next },
      {
        onSuccess: () =>
          next === "all_auto"
            ? autoIds.current.add(id)
            : autoIds.current.delete(id),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  const submit = async (e?: FormEvent) => {
    e?.preventDefault();
    const value = text.trim();
    if (!value || running) return;
    setSending(true);
    setSendError(null);
    try {
      let id = selected;
      if (id == null) {
        const created = await create.mutateAsync();
        id = created.id;
        if (draftMode !== "confirm")
          await setPermission.mutateAsync({ id, mode: draftMode });
        if (draftMode === "all_auto") autoIds.current.add(id);
        setSelected(id);
      }
      await unwrap(
        aiApi.POST("/ai/conversations/{conversationId}/messages", {
          params: { path: { conversationId: id } },
          body: { text: value },
        }),
      );
      setText("");
      qc.invalidateQueries({ queryKey: aiKeys.conversation(id) });
    } catch (err) {
      setSendError(errorMessage(err));
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  };

  const startNew = () => {
    setSelected(null);
    setDraftMode("confirm");
    setText("");
  };

  const current = conversations.find((c) => c.id === selected);
  const removeCurrent = async () =>
    current &&
    (await confirmAction({
      title: `${t("Delete")}“${conversationTitle(current.title)}”？`,
    })) &&
    remove.mutate(current.id, {
      onSuccess: () => {
        setSelected(conversations.find((c) => c.id !== current.id)?.id ?? null);
        qc.invalidateQueries({
          queryKey: aiKeys.hostConversations(host.id),
        });
      },
    });

  return (
    <div className="hagent">
      <aside className="hagent-list" aria-label={t("Conversations")}>
        <button type="button" className="xc-btn small" onClick={startNew}>
          <Plus size={14} /> {t("New conversation")}
        </button>
        {conversations.length === 0 ? (
          <p className="xc-muted hagent-none">{t("No conversations yet")}</p>
        ) : (
          <ul>
            {conversations.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  className={c.id === selected ? "active" : undefined}
                  aria-current={c.id === selected ? "true" : undefined}
                  onClick={() => setSelected(c.id)}
                >
                  <span>{conversationTitle(c.title)}</span>
                  <small>{relativeTime(c.updatedAt, language)}</small>
                </button>
              </li>
            ))}
          </ul>
        )}
      </aside>

      <section className="hagent-chat">
        <div className="hagent-head">
          <select
            className="xc-select hagent-picker"
            aria-label={t("Conversations")}
            value={selected ?? ""}
            onChange={(e) =>
              e.target.value ? setSelected(Number(e.target.value)) : startNew()
            }
          >
            <option value="">{t("New conversation")}</option>
            {conversations.map((c) => (
              <option key={c.id} value={c.id}>
                {conversationTitle(c.title)}
              </option>
            ))}
          </select>
          <strong className="hagent-title">
            {selected && detail.data
              ? conversationTitle(detail.data.conversation.title)
              : current
                ? conversationTitle(current.title)
                : t("New conversation")}
          </strong>
          {current && (
            <button
              type="button"
              className="ai-icon-btn"
              title={t("Delete")}
              aria-label={`${t("Delete")} ${conversationTitle(current.title)}`}
              onClick={() => void removeCurrent()}
            >
              <Trash2 size={15} />
            </button>
          )}
        </div>

        <div className="hagent-body ai-body" ref={bodyRef} aria-live="polite">
          {noModel ? (
            <div className="ai-empty">
              <Bot size={26} />
              <strong>{t("No Agent model yet")}</strong>
              <p>
                {t("Choose one in")}{" "}
                <Link to="/settings/assistant">{t("Settings → AI")}</Link>
              </p>
            </div>
          ) : selected && detail.isPending ? (
            <Loading />
          ) : items.length === 0 && !running ? (
            <div className="ai-empty">
              <Bot size={26} />
              <strong>{t("What should I do on this machine?")}</strong>
              <p>
                {t(
                  "AI runs on the panel and works through the agent on this machine. Commands wait for you to confirm by default.",
                )}
              </p>
              <div className="hagent-examples">
                {EXAMPLES.map((e) => (
                  <button
                    key={e}
                    type="button"
                    className="xc-btn small"
                    onClick={() => setText(t(e))}
                  >
                    {t(e)}
                  </button>
                ))}
              </div>
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
          {(liveError || sendError) && (
            <p className="xc-error-text ai-error">{liveError ?? sendError}</p>
          )}
        </div>

        {!host.online && (
          <p className="hagent-offline" role="status">
            {t("This machine is offline. Commands will fail until it is back.")}
          </p>
        )}
        <div className="hagent-bar">
          <label className="hagent-mode">
            <span>{t("Permission")}</span>
            <select
              className="xc-select"
              value={mode}
              disabled={setPermission.isPending}
              onChange={(e) =>
                void changeMode(e.target.value as HostAgentPermission)
              }
            >
              {PERMISSIONS.map((p) => (
                <option key={p.value} value={p.value}>
                  {t(p.label)}
                </option>
              ))}
            </select>
          </label>
        </div>
        <form className="ai-composer hagent-composer" onSubmit={submit}>
          <textarea
            rows={2}
            value={text}
            disabled={noModel}
            placeholder={t("Tell the Agent what to do on this machine")}
            aria-label={t("Message")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
          />
          {running && selected ? (
            <button
              type="button"
              className="ai-send stop"
              title={t("Stop")}
              aria-label={t("Stop")}
              onClick={() => stop.mutate(selected)}
            >
              <Square size={13} />
            </button>
          ) : (
            <button
              className="ai-send"
              title={t("Send")}
              aria-label={t("Send")}
              disabled={!text.trim() || noModel || sending}
            >
              <ArrowUp size={16} />
            </button>
          )}
        </form>
      </section>
    </div>
  );
}
