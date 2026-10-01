import { useEffect, useRef, useState, type FormEvent } from "react";
import { Trash2 } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import Switch from "../../../components/ui/Switch";
import { Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import {
  isNotLive,
  useMemories,
  useMemoryMutations,
  type AiMemory,
} from "../api";

/**
 * 设置 → AI 的“记忆”（B61）。面板 AI 和 Agent 都读，只有面板 AI 能改，
 * 远程 AI 看不到。地址带 #ai-memory 时滚到这里（浮窗里的“已记住”链接）。
 */
export default function MemoryCard() {
  const t = useT();
  const memories = useMemories();
  const ops = useMemoryMutations();
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    if (memories.data && location.hash === "#ai-memory")
      ref.current?.scrollIntoView?.({ block: "start" });
  }, [memories.data]);

  const add = (e: FormEvent) => {
    e.preventDefault();
    if (!text.trim()) return;
    setError("");
    ops.add.mutate(text.trim(), {
      onSuccess: () => setText(""),
      onError: (err) => setError(errorMessage(err)),
    });
  };

  return (
    <section className="xc-card ai-memory" id="ai-memory" ref={ref}>
      <div className="xc-card-head">
        <h2>{t("AI memory")}</h2>
        {memories.data && (
          <Switch
            checked={memories.data.enabled}
            label={t("Turn on memory")}
            onChange={(enabled) => ops.setEnabled.mutate(enabled)}
          />
        )}
      </div>
      <p className="xc-muted">
        {t(
          "The panel AI and agents read these. Only the panel AI can change them. Remote AI cannot see them.",
        )}
      </p>
      {memories.isPending ? (
        <Loading />
      ) : memories.isError ? (
        <p className="xc-muted">
          {isNotLive(memories.error)
            ? t("Not live yet")
            : errorMessage(memories.error)}
        </p>
      ) : (
        <>
          {memories.data.items.length === 0 ? (
            <p className="xc-muted">{t("Nothing remembered yet.")}</p>
          ) : (
            <ul className="xc-list ai-memory-list">
              {memories.data.items.map((m) => (
                <MemoryRow key={m.id} memory={m} />
              ))}
            </ul>
          )}
          <form className="ai-memory-add" onSubmit={add}>
            <input
              className="xc-input"
              value={text}
              maxLength={500}
              placeholder={t("Add a memory")}
              aria-label={t("Add a memory")}
              onChange={(e) => setText(e.target.value)}
            />
            <button
              className="xc-btn"
              disabled={!text.trim() || ops.add.isPending}
            >
              {t("Add")}
            </button>
          </form>
          {error && <p className="xc-error-text">{error}</p>}
          <small className="xc-muted">
            {memories.data.usedChars} / {memories.data.limitChars}{" "}
            {t("characters")}
          </small>
        </>
      )}
    </section>
  );
}

function MemoryRow({ memory }: { memory: AiMemory }) {
  const t = useT();
  const ops = useMemoryMutations();
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState("");
  const save = () => {
    if (draft === null) return;
    const text = draft.trim();
    if (!text || text === memory.text) {
      setDraft(null);
      return;
    }
    ops.update.mutate(
      { id: memory.id, text },
      {
        onSuccess: () => {
          setDraft(null);
          setError("");
        },
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };
  return (
    <li className="ai-memory-row">
      <div className="ai-memory-text">
        {draft === null ? (
          <button
            type="button"
            className="ai-memory-edit"
            title={t("Edit")}
            onClick={() => setDraft(memory.text)}
          >
            {memory.text}
          </button>
        ) : (
          <input
            className="xc-input"
            autoFocus
            maxLength={500}
            value={draft}
            aria-label={t("Edit memory")}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={save}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                save();
              }
              if (e.key === "Escape") {
                setDraft(null);
                setError("");
              }
            }}
          />
        )}
        {error && <small className="xc-error-text">{error}</small>}
      </div>
      {memory.source === "ai" && (
        <span className="xc-badge">{t("Saved by AI")}</span>
      )}
      <button
        type="button"
        className="icon-button"
        aria-label={`${t("Delete")}: ${memory.text}`}
        title={t("Delete")}
        onClick={async () => {
          if (
            await confirmAction({
              title: t("Delete this memory?"),
              description: memory.text,
              confirmLabel: t("Delete"),
            })
          )
            ops.remove.mutate(memory.id);
        }}
      >
        <Trash2 size={14} />
      </button>
    </li>
  );
}
