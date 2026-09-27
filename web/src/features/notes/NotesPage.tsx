import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { Archive, NotebookPen, Pin, Plus, Search, X } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useCreateNote, useNotes, useTags, type NoteSummary } from "./api";
import NoteEditor from "./components/NoteEditor";
import { noteTitle, snippetParts } from "./logic";

export default function NotesPage() {
  const t = useT();
  const navigate = useNavigate();
  const { noteId } = useParams();
  const [search, setSearch] = useSearchParams();
  const q = search.get("q") ?? "";
  const tag = search.get("tag") ?? "";
  const archived = search.get("archived") === "1";
  const [input, setInput] = useState(q);
  const searchRef = useRef<HTMLInputElement>(null);
  const create = useCreateNote();
  const tags = useTags();
  const notes = useNotes({ q, tag, archived });
  const id = noteId ? Number(noteId) : null;

  // 地址栏的 q 变了（比如后退），同步到输入框。
  useEffect(() => {
    setInput((current) => (current.trim() === q ? current : q));
  }, [q]);

  // 搜索框输入停 250 毫秒后更新地址栏，列表跟着刷新。
  useEffect(() => {
    if (input.trim() === q) return;
    const timer = setTimeout(() => setParam("q", input.trim()), 250);
    return () => clearTimeout(timer);
  }, [input]);

  const setParam = (name: string, value: string) =>
    setSearch(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(name, value);
        else next.delete(name);
        return next;
      },
      { replace: true },
    );
  const query = (() => {
    const p = new URLSearchParams(search);
    p.delete("new");
    p.delete("focus");
    const s = p.toString();
    return s ? `?${s}` : "";
  })();

  const newNote = () =>
    create.mutate(
      { tags: tag ? [tag] : [] },
      { onSuccess: (note) => navigate(`/notes/${note.id}${query}`) },
    );

  // 命令面板：/notes?new=1 新建，/notes?focus=search 聚焦搜索框。
  const handled = useRef(false);
  useEffect(() => {
    if (search.get("new") !== "1") handled.current = false;
    else if (!handled.current) {
      handled.current = true;
      setParam("new", "");
      newNote();
    }
    if (search.get("focus") === "search") {
      setParam("focus", "");
      searchRef.current?.focus();
    }
  }, [search]);

  const items = notes.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <div className={`notes-layout ${id ? "has-note" : ""}`}>
      <aside className="notes-sidebar">
        <div className="notes-sidebar-head">
          <label className="notes-search">
            <Search size={14} />
            <input
              ref={searchRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={t("Search notes")}
              aria-label={t("Search notes")}
            />
            {input && (
              <button aria-label={t("Clear")} onClick={() => setInput("")}>
                <X size={13} />
              </button>
            )}
          </label>
          <button
            className="xc-btn primary small"
            onClick={newNote}
            disabled={create.isPending}
          >
            <Plus size={14} />{" "}
            <span className="notes-btn-text">{t("New")}</span>
          </button>
        </div>
        <div className="notes-tags" role="group" aria-label={t("Tags")}>
          <button
            className={!tag && !archived ? "on" : ""}
            onClick={() => setSearch(q ? { q } : {}, { replace: true })}
          >
            {t("All")}
          </button>
          {tags.data?.map((tc) => (
            <button
              key={tc.tag}
              className={tag === tc.tag ? "on" : ""}
              aria-pressed={tag === tc.tag}
              onClick={() => setParam("tag", tag === tc.tag ? "" : tc.tag)}
            >
              #{tc.tag} <small>{tc.count}</small>
            </button>
          ))}
          <button
            className={archived ? "on" : ""}
            aria-pressed={archived}
            onClick={() => setParam("archived", archived ? "" : "1")}
          >
            <Archive size={12} /> {t("Archived")}
          </button>
        </div>
        <div className="notes-list" role="list">
          {notes.isPending ? (
            <Loading />
          ) : notes.isError ? (
            <ErrorState error={notes.error} onRetry={() => notes.refetch()} />
          ) : items.length === 0 ? (
            <EmptyState
              title={
                q
                  ? t("No matching notes")
                  : archived
                    ? t("No archived notes")
                    : t("No notes yet")
              }
              icon={<NotebookPen size={26} />}
            />
          ) : (
            items.map((n) => (
              <NoteItem
                key={n.id}
                note={n}
                active={n.id === id}
                query={query}
              />
            ))
          )}
          {notes.hasNextPage && (
            <button
              className="xc-btn ghost small notes-more"
              disabled={notes.isFetchingNextPage}
              onClick={() => notes.fetchNextPage()}
            >
              {t("Load more")}
            </button>
          )}
        </div>
      </aside>
      <main className="notes-main">
        {id ? (
          <NoteEditor id={id} backTo={`/notes${query}`} />
        ) : (
          <EmptyState
            title={t("Pick a note or start a new one")}
            icon={<NotebookPen size={28} />}
          >
            <button className="xc-btn small" onClick={newNote}>
              <Plus size={14} /> {t("New note")}
            </button>
          </EmptyState>
        )}
      </main>
    </div>
  );
}

function NoteItem({
  note,
  active,
  query,
}: {
  note: NoteSummary;
  active: boolean;
  query: string;
}) {
  const t = useT();
  const language = useLanguage();
  const title = noteTitle(note.title, note.excerpt) || t("Untitled note");
  return (
    <Link
      to={`/notes/${note.id}${query}`}
      role="listitem"
      className={`notes-item${active ? " active" : ""}`}
      aria-current={active ? "page" : undefined}
    >
      <div className="notes-item-head">
        {note.pinned && (
          <Pin size={12} className="notes-pin" aria-label={t("Pinned")} />
        )}
        <strong>{title}</strong>
        <time dateTime={note.updatedAt}>
          {relativeTime(note.updatedAt, language)}
        </time>
      </div>
      <p>
        {note.snippet
          ? snippetParts(note.snippet).map((part, i) =>
              part.hit ? (
                <mark key={i}>{part.text}</mark>
              ) : (
                <span key={i}>{part.text}</span>
              ),
            )
          : note.excerpt || <span className="xc-muted">{t("Empty note")}</span>}
      </p>
      {note.tags.length > 0 && (
        <div className="notes-item-tags">
          {note.tags.map((tag) => (
            <span key={tag}>#{tag}</span>
          ))}
        </div>
      )}
    </Link>
  );
}
