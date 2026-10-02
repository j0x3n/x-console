import { Link } from "react-router";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import {
  Archive,
  Palette,
  Pin,
  PinOff,
  StickyNote,
  Trash2,
  X,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { isNotLive, unwrap } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import { toggleTask } from "../../components/markdown/mdparse";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  notesApi,
  notesKeys,
  useCreateNote,
  useDeleteNote,
  useNoteCounts,
  useNotes,
  useTags,
  useUpdateNote,
  type NoteColor,
  type NoteSummary,
} from "./api";
import ColorPicker from "./ColorPicker";
import NoteEditor from "./components/NoteEditor";
import { noteBgClass } from "./noteColors";
import { tagColor } from "./tagColor";
import QuoteButton from "./QuoteDialog";
import { QUOTE_TAG } from "../overview/quote";

/**
 * 便签瀑布流（B73），和 Google Keep 一样。顶部“记个便签…”就地展开写，
 * 下面先是置顶的，再是其他的。点卡片在弹窗里编辑。
 * tag 有值时只列这个标签的便签，新写的便签自动带上这个标签。
 */
export default function MemoBoard({
  tag = "",
  hidden = false,
}: {
  tag?: string;
  hidden?: boolean;
}) {
  const t = useT();
  const counts = useNoteCounts();
  const memos = useNotes({
    q: "",
    tag,
    archived: false,
    pinned: false,
    hidden,
    kind: "memo",
  });
  const [openId, setOpenId] = useState<number | null>(null);
  // /notes/counts 回 501 说明后端还没有便签（kind 参数会被忽略），不能列出来。
  if (counts.isError && isNotLive(counts.error))
    return (
      <div className="notes-memos">
        <NotLive name={t("Memos")} icon={<StickyNote size={28} />} />
      </div>
    );
  const all = memos.data?.pages.flatMap((p) => p.items) ?? [];
  // B89：便签总览里不放“名言”，它们只在名言标签下和今日页出现
  const quotes = tag ? [] : all.filter((m) => m.tags.includes(QUOTE_TAG));
  const items = tag ? all : all.filter((m) => !m.tags.includes(QUOTE_TAG));
  const pinned = items.filter((m) => m.pinned);
  const others = items.filter((m) => !m.pinned);
  return (
    <div className="notes-memos">
      <MemoComposer tag={tag} hidden={hidden} />
      {!hidden && (!tag || tag === QUOTE_TAG) && (
        <div className="notes-memos-bar">
          <QuoteButton count={tag ? all.length : quotes.length} />
          {!tag && quotes.length > 0 && (
            <Link
              className="xc-btn small ghost"
              to={`/notes?tag=${encodeURIComponent(QUOTE_TAG)}`}
            >
              #{QUOTE_TAG} {quotes.length}
            </Link>
          )}
        </div>
      )}
      {memos.isPending ? (
        <Loading />
      ) : memos.isError ? (
        <ErrorState error={memos.error} onRetry={() => memos.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          title={tag ? t("No memos with this tag") : t("No memos yet")}
          icon={<StickyNote size={26} />}
        >
          <span>{t("Quick notes from the top bar are saved here.")}</span>
        </EmptyState>
      ) : (
        <>
          {pinned.length > 0 && (
            <>
              <h2 className="notes-memos-label">{t("Pinned")}</h2>
              <div className="notes-masonry">
                {pinned.map((m) => (
                  <MemoCard
                    key={m.id}
                    memo={m}
                    onOpen={() => setOpenId(m.id)}
                  />
                ))}
              </div>
            </>
          )}
          {others.length > 0 && (
            <>
              {pinned.length > 0 && (
                <h2 className="notes-memos-label">{t("Others")}</h2>
              )}
              <div className="notes-masonry">
                {others.map((m) => (
                  <MemoCard
                    key={m.id}
                    memo={m}
                    onOpen={() => setOpenId(m.id)}
                  />
                ))}
              </div>
            </>
          )}
          {memos.hasNextPage && (
            <button
              className="xc-btn ghost small notes-more"
              disabled={memos.isFetchingNextPage}
              onClick={() => memos.fetchNextPage()}
            >
              {t("Load more")}
            </button>
          )}
        </>
      )}
      {openId != null && (
        <MemoDialog id={openId} onClose={() => setOpenId(null)} />
      )}
    </div>
  );
}

/** 顶部的“记个便签…”。点开就地展开标题和正文，点外面或 Ctrl+Enter 保存。 */
function MemoComposer({ tag, hidden }: { tag: string; hidden: boolean }) {
  const t = useT();
  const create = useCreateNote();
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [color, setColor] = useState<NoteColor>("");
  const [colors, setColors] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const stateRef = useRef({ title, body, color });
  stateRef.current = { title, body, color };

  const save = () => {
    const s = stateRef.current;
    if (!s.title.trim() && !s.body.trim()) {
      setOpen(false);
      return;
    }
    create.mutate(
      {
        kind: "memo",
        title: s.title.trim(),
        body: s.body,
        color: s.color,
        tags: tag ? [tag] : [],
        hidden: hidden || undefined,
        quick: true,
      },
      {
        onSuccess: () => {
          setTitle("");
          setBody("");
          setColor("");
          setOpen(false);
        },
      },
    );
  };

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) save();
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);

  if (!open)
    return (
      <button
        type="button"
        className="notes-memo-composer collapsed"
        onClick={() => setOpen(true)}
      >
        <StickyNote size={15} />
        <span>{t("Take a memo…")}</span>
      </button>
    );
  return (
    <div
      ref={ref}
      className={`notes-memo-composer ${noteBgClass(color)}`}
      onKeyDown={(e) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) save();
        if (e.key === "Escape") save();
      }}
    >
      <input
        className="notes-memo-title"
        value={title}
        placeholder={t("Title")}
        aria-label={t("Title")}
        onChange={(e) => setTitle(e.target.value)}
      />
      <textarea
        className="notes-memo-body"
        autoFocus
        rows={3}
        value={body}
        placeholder={t("Take a memo…")}
        aria-label={t("Memo")}
        onChange={(e) => setBody(e.target.value)}
      />
      <div className="notes-memo-composer-bar">
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Background")}
          aria-label={t("Background")}
          aria-expanded={colors}
          onClick={() => setColors((v) => !v)}
        >
          <Palette size={14} />
        </button>
        {tag && <span className="notes-tag">#{tag}</span>}
        <span className="xc-spacer" />
        <span className="xc-muted notes-hint">⌘/Ctrl + Enter</span>
        <button
          type="button"
          className="xc-btn small"
          disabled={create.isPending}
          onClick={save}
        >
          {t("Close")}
        </button>
      </div>
      {colors && <ColorPicker value={color} onChange={setColor} />}
    </div>
  );
}

/** 瀑布流里的一张便签。鼠标移上去出现置顶、颜色、归档、删除。 */
function MemoCard({ memo, onOpen }: { memo: NoteSummary; onOpen: () => void }) {
  const t = useT();
  const qc = useQueryClient();
  const update = useUpdateNote();
  const remove = useDeleteNote();
  const tags = useTags();
  const [colors, setColors] = useState(false);
  const body = memo.body ?? memo.excerpt;
  const toggle = async (index: number) => {
    // 正文截断了就先取全文，免得把后面的内容存丢
    let full = memo.body ?? "";
    if (memo.bodyTruncated || memo.body == null) {
      const note = await qc.fetchQuery({
        queryKey: notesKeys.note(memo.id),
        queryFn: () =>
          unwrap(
            notesApi.GET("/notes/{noteId}", {
              params: { path: { noteId: memo.id } },
            }),
          ),
      });
      full = note.body;
    }
    update.mutate({ id: memo.id, body: { body: toggleTask(full, index) } });
  };
  return (
    <article
      className={`notes-memo ${noteBgClass(memo.color)}`}
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" && e.target === e.currentTarget) onOpen();
      }}
    >
      {memo.pinned && (
        <Pin size={12} className="notes-memo-pin" aria-label={t("Pinned")} />
      )}
      {memo.title.trim() && <h3>{memo.title}</h3>}
      <div className="notes-memo-text">
        <Markdown source={body} onToggleTask={(i) => void toggle(i)} />
      </div>
      {memo.tags.length > 0 && (
        <div className="notes-memo-tags">
          {memo.tags.map((tag) => (
            <span
              key={tag}
              className="notes-item-tag"
              style={
                {
                  "--tag": tagColor(
                    tag,
                    tags.data?.find((x) => x.tag === tag)?.color,
                  ),
                } as CSSProperties
              }
            >
              #{tag}
            </span>
          ))}
        </div>
      )}
      <div
        className={`notes-memo-actions${colors ? " show" : ""}`}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          type="button"
          title={memo.pinned ? t("Unpin") : t("Pin")}
          aria-label={memo.pinned ? t("Unpin") : t("Pin")}
          onClick={() =>
            update.mutate({ id: memo.id, body: { pinned: !memo.pinned } })
          }
        >
          {memo.pinned ? <PinOff size={14} /> : <Pin size={14} />}
        </button>
        <button
          type="button"
          title={t("Background")}
          aria-label={t("Background")}
          aria-expanded={colors}
          onClick={() => setColors((v) => !v)}
        >
          <Palette size={14} />
        </button>
        <button
          type="button"
          title={t("Archive")}
          aria-label={t("Archive")}
          onClick={() =>
            update.mutate(
              { id: memo.id, body: { archived: true } },
              { onSuccess: () => toast(t("Archived")) },
            )
          }
        >
          <Archive size={14} />
        </button>
        <button
          type="button"
          title={t("Delete")}
          aria-label={t("Delete")}
          onClick={async () => {
            if (
              await confirmAction({
                title: t("Delete this memo?"),
                description: t("This cannot be undone."),
              })
            )
              remove.mutate(memo.id, { onSuccess: () => toast(t("Deleted")) });
          }}
        >
          <Trash2 size={14} />
        </button>
      </div>
      {colors && (
        <div className="notes-memo-colors" onClick={(e) => e.stopPropagation()}>
          <ColorPicker
            value={memo.color}
            onChange={(color) => {
              update.mutate({ id: memo.id, body: { color } });
              setColors(false);
            }}
          />
        </div>
      )}
    </article>
  );
}

/** 点便签打开的弹窗：就是笔记编辑器，紧凑一些。 */
function MemoDialog({ id, onClose }: { id: number; onClose: () => void }) {
  const t = useT();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="modal-backdrop notes-memo-backdrop" onClick={onClose}>
      <div
        className="notes-memo-dialog"
        role="dialog"
        aria-label={t("Memo")}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          type="button"
          className="icon-button notes-memo-close"
          aria-label={t("Close")}
          title={t("Close")}
          onClick={onClose}
        >
          <X size={15} />
        </button>
        <NoteEditor id={id} backTo="/notes" floating onClosed={onClose} />
      </div>
    </div>
  );
}
