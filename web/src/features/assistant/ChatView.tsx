import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router";
import { Plus, Send, Settings2 } from "lucide-react";
import { withElevation } from "../../auth/elevation";
import { errorMessage } from "../../api/client";
import { onServerEvent } from "../../api/events";
import { toast } from "../../hooks/useToast";
import { ErrorState, Loading } from "../../components/ui/States";
import {
  useAISettings,
  useConversation,
  useConversations,
  useCreateConversation,
  useDecideAction,
  useSaveAISettings,
  useSendMessage,
  type PendingAction,
} from "./api";

function MessageContent({
  content,
}: {
  content: Array<Record<string, unknown>>;
}) {
  return (
    <div className="assistant-blocks">
      {content.map((block, index) => {
        if (block.type === "text")
          return <p key={index}>{String(block.text ?? "")}</p>;
        if (block.type === "tool_use")
          return (
            <p key={index} className="xc-muted">
              请求动作：{String(block.name ?? "")}
            </p>
          );
        if (block.type === "tool_result")
          return (
            <p key={index} className="xc-muted">
              动作结果：{String(block.content ?? "")}
            </p>
          );
        return null;
      })}
    </div>
  );
}

function ActionCard({ action }: { action: PendingAction }) {
  const decide = useDecideAction();
  const act = async (approved: boolean) => {
    try {
      await withElevation(() =>
        decide.mutateAsync({ id: action.id, approved }),
      );
      toast(approved ? "已确认动作" : "已拒绝动作");
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <div className="xc-card assistant-action">
      <strong>待确认：{action.action}</strong>
      <pre>{JSON.stringify(action.input, null, 2)}</pre>
      <div className="xc-row">
        <button
          className="xc-btn small primary"
          disabled={decide.isPending}
          onClick={() => void act(true)}
        >
          确认
        </button>
        <button
          className="xc-btn small"
          disabled={decide.isPending}
          onClick={() => void act(false)}
        >
          拒绝
        </button>
      </div>
    </div>
  );
}

function SettingsForm({ close }: { close: () => void }) {
  const settings = useAISettings();
  const save = useSaveAISettings();
  const [model, setModel] = useState("");
  const [apiKey, setApiKey] = useState("");
  useEffect(() => {
    if (settings.data) setModel(settings.data.model);
  }, [settings.data]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await withElevation(() =>
        save.mutateAsync({
          model: model.trim(),
          apiKey: apiKey.trim() || undefined,
        }),
      );
      setApiKey("");
      close();
      toast("AI 设置已保存");
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <form className="xc-card assistant-settings" onSubmit={submit}>
      <h3>AI 设置</h3>
      <label className="xc-field">
        <span>模型</span>
        <input
          className="xc-input"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          required
        />
      </label>
      <label className="xc-field">
        <span>Anthropic API Key</span>
        <input
          className="xc-input"
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={
            settings.data?.configured ? "已设置。留空则不修改" : "输入 API Key"
          }
        />
      </label>
      <div className="xc-row">
        <button className="xc-btn small primary" disabled={save.isPending}>
          保存
        </button>
        <button type="button" className="xc-btn small" onClick={close}>
          取消
        </button>
      </div>
    </form>
  );
}

export default function ChatView({ compact = false }: { compact?: boolean }) {
  const conversations = useConversations();
  const create = useCreateConversation();
  const [selected, setSelected] = useState("");
  const [draft, setDraft] = useState("");
  const [delta, setDelta] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [failure, setFailure] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const detail = useConversation(selected);
  const settings = useAISettings();
  const send = useSendMessage();

  useEffect(() => {
    if (!selected && conversations.data?.length)
      setSelected(conversations.data[0].id);
  }, [conversations.data, selected]);
  useEffect(() => {
    return onServerEvent((event) => {
      const data = event.data as {
        conversationId?: string;
        text?: string;
        message?: string;
      };
      if (data?.conversationId !== selected) return;
      if (event.topic === "ai.delta") {
        setDelta((old) => old + (data.text ?? ""));
        setStreaming(true);
      }
      if (event.topic === "ai.message_done") {
        setDelta("");
        setStreaming(false);
      }
      if (event.topic === "ai.action_pending") {
        setDelta("");
        setStreaming(false);
      }
      if (event.topic === "ai.error") {
        setFailure(data.message ?? "回复失败");
        setStreaming(false);
      }
    });
  }, [selected]);
  const newConversation = async () => {
    try {
      const value = await create.mutateAsync();
      setSelected(value.id);
      setDelta("");
      setFailure("");
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const text = draft.trim();
    if (!selected || !text) return;
    try {
      setStreaming(true);
      setFailure("");
      await send.mutateAsync({ id: selected, text });
      setDraft("");
    } catch (error) {
      setStreaming(false);
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <div className={`assistant-chat ${compact ? "compact" : ""}`}>
      <aside className="assistant-conversations">
        <button
          className="xc-btn small"
          onClick={() => void newConversation()}
          disabled={create.isPending}
        >
          <Plus size={14} /> 新对话
        </button>
        {conversations.isPending && <Loading />}
        {conversations.isError && (
          <ErrorState
            error={conversations.error}
            onRetry={() => conversations.refetch()}
          />
        )}
        {conversations.data?.map((item) => (
          <button
            key={item.id}
            className={item.id === selected ? "active" : ""}
            onClick={() => {
              setSelected(item.id);
              setDelta("");
              setFailure("");
            }}
          >
            {item.title}
          </button>
        ))}
        <button
          className="xc-btn small ghost"
          onClick={() => setShowSettings(!showSettings)}
        >
          <Settings2 size={14} /> AI 设置
        </button>
      </aside>
      <section className="assistant-conversation">
        {showSettings && <SettingsForm close={() => setShowSettings(false)} />}
        {!settings.isPending && !settings.data?.configured && (
          <p className="xc-card assistant-notice">
            先在 AI 设置中填写 Anthropic API Key。
          </p>
        )}
        {!selected && (
          <div className="assistant-empty">新建对话后就能开始。</div>
        )}
        {selected && detail.isPending && <Loading />}
        {selected && detail.isError && (
          <ErrorState error={detail.error} onRetry={() => detail.refetch()} />
        )}
        {detail.data && (
          <div className="assistant-messages">
            {detail.data.messages.map((message) => (
              <article
                key={message.id}
                className={`assistant-message ${message.role}`}
              >
                <small>{message.role === "user" ? "你" : "AI 助手"}</small>
                <MessageContent content={message.content} />
              </article>
            ))}
            {detail.data.pendingActions
              .filter((action) => action.status === "pending")
              .map((action) => (
                <ActionCard key={action.id} action={action} />
              ))}
            {delta && (
              <article className="assistant-message assistant">
                <small>AI 助手</small>
                <p>{delta}</p>
              </article>
            )}
            {failure && <p className="xc-error-text">{failure}</p>}
          </div>
        )}
        {selected && (
          <form className="assistant-compose" onSubmit={submit}>
            <textarea
              className="xc-textarea"
              aria-label="消息"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="输入你想让助手做的事"
              rows={compact ? 2 : 3}
            />
            <button
              className="xc-btn primary"
              disabled={
                !draft.trim() ||
                send.isPending ||
                streaming ||
                !settings.data?.configured
              }
            >
              <Send size={14} /> 发送
            </button>
          </form>
        )}
        {!compact && (
          <p className="assistant-footer">
            自动执行只读动作。修改数据需要你确认。高危动作还要两步验证。{" "}
            <Link to="/automations">查看自动化</Link>
          </p>
        )}
      </section>
    </div>
  );
}
