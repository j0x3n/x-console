import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { Link, useLocation } from "react-router";
import {
  ArrowUp,
  FileText,
  Loader2,
  Paperclip,
  ShieldAlert,
  ShieldCheck,
  Square,
  X,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { errorMessage } from "../../../api/client";
import { ErrorState, Loading } from "../../../components/ui/States";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  aiKeys,
  attachmentUrl,
  uploadAttachment,
  useConversation,
  useModelSettings,
  useSendMessage,
  useSetPermission,
  useStopReply,
  useTools,
  type AiAttachment,
  type ChatSettings,
  type HostAgentPermission,
  type PageContext,
} from "../api";
import { buildTimeline, HOST_PERMISSIONS, HOST_TOOLS } from "../logic";
import { useAssistant } from "../store";
import AiMark from "./AiMark";
import ChatControls from "./ChatControls";
import Picker from "./Picker";
import Timeline from "./Timeline";

function pageContext(pathname: string): PageContext {
  const h1 = document.querySelector("#main .xc-page-title h1, #main h1");
  return { path: pathname, title: h1?.textContent?.trim() ?? "" };
}

/**
 * B183：AI 窗口右边的一个对话。面板对话和服务器 Agent 会话共用：
 * 服务器会话没有附件、页面信息和模型设置，换成服务器 Agent 的权限。
 * conversationId 为空时是新的面板对话。
 */
export default function ConversationView({
  conversationId,
  noKey,
}: {
  conversationId: number | null;
  /** 还没选 Agent 模型 */
  noKey: boolean;
}) {
  const t = useT();
  const location = useLocation();
  const qc = useQueryClient();
  const setConversation = useAssistant((s) => s.setConversation);
  const streaming = useAssistant((s) =>
    conversationId ? s.streaming[conversationId] : undefined,
  );
  const liveError = useAssistant((s) =>
    conversationId ? s.errors[conversationId] : undefined,
  );
  const models = useModelSettings();
  const tools = useTools();
  const detail = useConversation(conversationId);
  const send = useSendMessage();
  const stop = useStopReply();
  const setHostPermission = useSetPermission();
  const current = detail.data?.conversation;
  const host = !!current?.hostId;

  // B60：这个对话的权限、模型和思考程度。还没有对话时先记在这里。
  const [draft, setDraft] = useState<ChatSettings | null>(null);
  const chat: ChatSettings & { until?: string | null } =
    conversationId && current
      ? {
          permission: current.panelPermission ?? "manual",
          model: current.model ?? "",
          effort: current.effort ?? "",
          until: current.panelPermissionUntil,
        }
      : (draft ?? {
          permission: models.data?.defaultPermission ?? "manual",
          model: "",
          effort: "",
        });

  const [text, setText] = useState("");
  const [attachments, setAttachments] = useState<AiAttachment[]>([]);
  const [uploading, setUploading] = useState(0);
  const fileRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // 当前对话被删了或不存在时，回到新对话。
  useEffect(() => {
    if (detail.isError) setConversation(null);
  }, [detail.isError]);

  const running = !!detail.data?.running || send.isPending;
  const hostTools = useMemo(
    () => HOST_TOOLS.map((tool) => ({ ...tool, title: t(tool.title) })),
    [t],
  );
  const items = useMemo(
    () =>
      detail.data
        ? buildTimeline(
            detail.data.messages,
            detail.data.pendingActions,
            host ? hostTools : (tools.data ?? []),
            { running, streaming },
          )
        : [],
    [detail.data, tools.data, hostTools, host, running, streaming],
  );

  // 有新内容时滚到底。
  useEffect(() => {
    const el = bodyRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items.length, streaming, conversationId]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [conversationId]);

  // B39：图片和文本文件先上传，发消息时带上 id。
  const addFiles = (files: File[]) => {
    const room = 10 - attachments.length - uploading;
    if (files.length > room)
      toast({ message: t("Up to 10 attachments per message"), tone: "error" });
    for (const file of files.slice(0, Math.max(room, 0))) {
      setUploading((n) => n + 1);
      uploadAttachment(file)
        .then((a) => setAttachments((list) => [...list, a]))
        .catch((err) => toast({ message: errorMessage(err), tone: "error" }))
        .finally(() => setUploading((n) => n - 1));
    }
  };

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    const value = text.trim();
    if ((!value && !attachments.length) || running || uploading) return;
    send.mutate(
      host
        ? { conversationId, text: value }
        : {
            conversationId,
            text: value || t("Please look at the attachments."),
            context: pageContext(location.pathname),
            attachmentIds: attachments.map((a) => a.id),
            settings: conversationId ? undefined : chat,
          },
      {
        onSuccess: (id) => {
          setText("");
          setAttachments([]);
          setDraft(null);
          if (id !== conversationId) setConversation(id);
          qc.invalidateQueries({ queryKey: aiKeys.sessions });
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

  const changeHostMode = async (next: HostAgentPermission) => {
    if (!conversationId || next === current?.permission) return;
    if (
      next === "all_auto" &&
      !(await confirmAction({
        title: t("Run everything without asking?"),
        description: t(
          "Dangerous commands still need your confirmation. It goes back to confirming each step after 2 hours without a new message.",
        ),
        confirmLabel: t("Run everything"),
      }))
    )
      return;
    setHostPermission.mutate(
      { id: conversationId, mode: next },
      {
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  const waiting = items.some(
    (i) => i.kind === "action" && i.status === "waiting",
  );

  return (
    <>
      <div className="ai-body" ref={bodyRef} aria-live="polite">
        {noKey && !host ? (
          <div className="ai-empty">
            <AiMark size={22} tile />
            <strong>还没有选 Agent 模型</strong>
            <p>
              去 <Link to="/settings/assistant">设置 → AI</Link>{" "}
              添加供应商，再选一个 Agent 模型就能用了。
            </p>
          </div>
        ) : conversationId && detail.isPending ? (
          <Loading />
        ) : detail.isError ? (
          <ErrorState error={detail.error} onRetry={() => detail.refetch()} />
        ) : items.length === 0 && !running ? (
          <div className="ai-empty">
            <AiMark size={22} tile />
            <strong>有什么要我做的？</strong>
            <p>
              我能查看和修改笔记、提醒、项目、习惯、云盘，也能看服务器状态。
            </p>
          </div>
        ) : (
          <>
            <Timeline items={items} />
            {running && !streaming && !waiting && (
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

      <form className="ai-input" onSubmit={submit}>
        {(attachments.length > 0 || uploading > 0) && (
          <div className="ai-pending-attachments">
            {attachments.map((a) => (
              <span key={a.id} className="ai-attach-chip" title={a.name}>
                {a.kind === "image" ? (
                  <img src={attachmentUrl(a.id)} alt="" />
                ) : (
                  <FileText size={13} />
                )}
                <span>{a.name}</span>
                <button
                  type="button"
                  className="ai-attach-remove"
                  aria-label={`${t("Remove attachment")} ${a.name}`}
                  onClick={() =>
                    setAttachments((list) => list.filter((x) => x.id !== a.id))
                  }
                >
                  <X size={12} />
                </button>
              </span>
            ))}
            {uploading > 0 && (
              <span className="ai-attach-chip" role="status">
                <Loader2 size={13} className="ai-spin" />
                <span>{t("Uploading…")}</span>
              </span>
            )}
          </div>
        )}
        <textarea
          ref={inputRef}
          rows={1}
          value={text}
          placeholder={t(
            host
              ? "Tell the Agent what to do on this machine"
              : "Ask or tell me what to do",
          )}
          aria-label={t("Message")}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          onPaste={(e) => {
            if (host) return;
            const files = Array.from(e.clipboardData.files);
            if (files.length) {
              e.preventDefault();
              addFiles(files);
            }
          }}
        />
        <div className="ai-composer-bar">
          {!host && (
            <>
              <input
                ref={fileRef}
                type="file"
                multiple
                hidden
                accept="image/png,image/jpeg,image/webp,image/gif,text/*,.md,.json,.yaml,.yml,.toml,.csv,.log,.go,.ts,.tsx,.js,.jsx,.py,.sh,.sql,.java,.rs,.c,.h,.cpp,.css,.html,.xml,.ini,.conf"
                onChange={(e) => {
                  addFiles(Array.from(e.target.files ?? []));
                  e.target.value = "";
                }}
              />
              <button
                type="button"
                className="ai-attach-btn"
                title={t("Attach images or text files")}
                aria-label={t("Attach images or text files")}
                onClick={() => fileRef.current?.click()}
              >
                <Paperclip size={15} />
              </button>
            </>
          )}
          <div className="ai-chips">
            {host ? (
              <Picker
                label={t("Permission")}
                icon={
                  current?.permission === "all_auto" ? (
                    <ShieldAlert size={13} />
                  ) : (
                    <ShieldCheck size={13} />
                  )
                }
                tone={current?.permission === "all_auto" ? "warn" : undefined}
                value={current?.permission ?? "confirm"}
                disabled={setHostPermission.isPending}
                options={HOST_PERMISSIONS.map((p) => ({
                  value: p.value,
                  label: t(p.label),
                }))}
                onChange={(v) => void changeHostMode(v as HostAgentPermission)}
              />
            ) : (
              !noKey && (
                <ChatControls
                  conversationId={conversationId}
                  value={chat}
                  defaultModel={models.data?.agent?.model ?? ""}
                  onDraft={setDraft}
                />
              )
            )}
          </div>
          {running && conversationId ? (
            <button
              type="button"
              className="ai-send stop"
              title={t("Stop")}
              aria-label={t("Stop")}
              onClick={() => stop.mutate(conversationId)}
            >
              <Square size={12} fill="currentColor" />
            </button>
          ) : (
            <button
              className="ai-send"
              title={t("Send")}
              aria-label={t("Send")}
              disabled={(!text.trim() && !attachments.length) || uploading > 0}
            >
              <ArrowUp size={16} />
            </button>
          )}
        </div>
      </form>
    </>
  );
}
