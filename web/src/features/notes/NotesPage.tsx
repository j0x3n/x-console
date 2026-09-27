import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import {
  Archive,
  Hash,
  NotebookPen,
  Notebook,
  Pin,
  Plus,
  Search,
  X,
} from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useCreateNote, useNotes, useTags, type NoteSummary } from "./api";
import NoteEditor from "./components/NoteEditor";
import {
  DATE_GROUP_LABELS,
  dateGroup,
  noteTitle,
  snippetParts,
  type DateGroup,
} from "./logic";

type View = "all" | "pinned" | "archived" | "tag";

export default function NotesPage() {
  const t = useT();
  const navigate = useNavigate();
  const { noteId } = useParams();
  const [search, setSearch] = useSearchParams();
  const q = search.get("q") ?? "";
  const tag = search.get("tag") ?? "";
  const archived = search.get("archived") === "1";
  const pinned = search.get("pinned") === "1";
  const view: View = tag ? "tag" : archived ? "archived" : pinned ? "pinned" : "all";
  const [input, setInput] = useState(q);
  const searchRef = useRef<HTMLInputElement>(null);
  const create = useCreateNote();
  const tags = useTags();
  const notes = useNotes({ q, tag, archived, pinned });
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
  const setView = (next: View, nextTag = "") =>
    setSearch(
      () => {
        const p = new URLSearchParams();
        if (q) p.set("q", q);
        if (next === "pinned") p.set("pinned", "1");
        if (next === "archived") p.set("archived", "1");
        if (next === "tag" && nextTag) p.set("tag", nextTag);
        return p;
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
      { tags: tag ? [tag] : [], pinned: pinned || undefined },
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
  const groups = useMemo(() => groupNotes(items, !!q, view), [items, q, view]);
  const viewTitle =
    view === "tag"
      ? `#${tag}`
      : view === "pinned"
        ? t("Pinned notes")
        : view === "archived"
          ? t("Archived")
          : t("All notes");

  const navItem = (key: string, active: boolean, onClick: () => void, icon: ReactNode, label: string, count?: number) => (
    <button key={key} className={active ? "active" : ""} aria-pressed={active} onClick={onClick}>
      {icon}
      <span>{label}</span>
      {count != null && <small>{count}</small>}
    </button>
  );

  const views = [
    navItem("all", view === "all", () => setView("all"), <Notebook size={15} />, t("All notes")),
    navItem("pinned", view === "pinned", () => setView("pinned"), <Pin size={15} />, t("Pinned notes")),
    navItem("archived", view === "archived", () => setView("archived"), <Archive size={15} />, t("Archived")),
  ];
  const tagItems =
    tags.data?.map((tc) =>
      navItem(
        `tag-${tc.tag}`,
        tag === tc.tag,
        () => setView(tag === tc.tag ? "all" : "tag", tc.tag),
        <Hash size={14} />,
        tc.tag,
        tc.count,
      ),
    ) ?? [];

  return (
    <div className={`notes-layout ${id ? "has-note" : ""}`}>
      <nav className="notes-nav" aria-label={t("Note categories")}>
        <div className="notes-nav-group">{views}</div>
        {tagItems.length > 0 && (
          <div className="notes-nav-group">
            <div className="notes-nav-label">{t("Tags")}</div>
            {tagItems}
          </div>
        )}
      </nav>
      <aside className="notes-list-pane">
        <div className="notes-list-head">
          <div className="notes-list-title">
            <h1>{viewTitle}</h1>
            <span>{items.length > 0 ? `${items.length}${notes.hasNextPage ? "+" : ""}` : ""}</span>
          </div>
          <button
            className="xc-btn primary small"
            onClick={newNote}
            disabled={create.isPending}
            title={t("New note")}
          >
            <Plus size={14} /> <span className="notes-btn-text">{t("New")}</span>
          </button>
        </div>
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
        <div className="notes-chips" role="group" aria-label={t("Tags")}>
          {views}
          {tagItems}
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
                  : view === "archived"
                    ? t("No archived notes")
                    : view === "pinned"
                      ? t("No pinned notes")
                      : t("No notes yet")
              }
              icon={<NotebookPen size={26} />}
            >
              {!q && view !== "archived" && (
                <button className="xc-btn small" onClick={newNote}>
                  <Plus size={14} /> {t("New note")}
                </button>
              )}
            </EmptyState>
          ) : (
            groups.map((g) => (
              <section key={g.key} className="notes-group">
                {g.label && <h2 className="notes-group-label">{t(g.label)}</h2>}
                {g.items.map((n) => (
                  <NoteItem key={n.id} note={n} active={n.id === id} query={query} />
                ))}
              </section>
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
          <div className="notes-blank">
            <NotebookPen size={30} />
            <strong>{t("Pick a note or start a new one")}</strong>
            <span>{t("Paste or drag images straight into a note.")}</span>
            <button className="xc-btn primary small" onClick={newNote}>
              <Plus size={14} /> {t("New note")}
            </button>
          </div>
        )}
      </main>
    </div>
  );
}

interface Group {
  key: string;
  label: string;
  items: NoteSummary[];
}

/** 置顶的在最上面一组，其余按更新时间分组。搜索时按相关度，不分组。 */
function groupNotes(items: NoteSummary[], searching: boolean, view: View): Group[] {
  if (searching) return [{ key: "search", label: "", items }];
  const groups: Group[] = [];
  const add = (key: string, label: string, note: NoteSummary) => {
    let g = groups.find((x) => x.key === key);
    if (!g) {
      g = { key, label, items: [] };
      groups.push(g);
    }
    g.items.push(note);
  };
  const now = new Date();
  for (const n of items) {
    if (n.pinned && view !== "pinned" && view !== "archived") add("pinned", "Pinned notes", n);
    else {
      const g: DateGroup = dateGroup(n.updatedAt, now);
      add(g, DATE_GROUP_LABELS[g], n);
    }
  }
  return groups;
}

function NoteItem({ note, active, query }: { note: NoteSummary; active: boolean; query: string }) {
  const t = useT();
  const language = useLanguage();
  const title = noteTitle(note.title, note.excerpt) || t("Untitled note");
  const body = note.title.trim() ? note.excerpt : note.excerpt.slice(title.length).trim();
  return (
    <Link
      to={`/notes/${note.id}${query}`}
      role="listitem"
      className={`notes-item${active ? " active" : ""}`}
      aria-current={active ? "page" : undefined}
    >
      <div className="notes-item-text">
        <strong>
          {note.pinned && <Pin size={11} className="notes-pin" aria-label={t("Pinned")} />}
          {title}
        </strong>
        <p>
          {note.snippet
            ? snippetParts(note.snippet).map((part, i) =>
                part.hit ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>,
              )
            : body || <span className="notes-item-empty">{t("No more text")}</span>}
        </p>
        <div className="notes-item-foot">
          <time dateTime={note.updatedAt}>{relativeTime(note.updatedAt, language)}</time>
          {note.tags.slice(0, 3).map((tag) => (
            <span key={tag} className="notes-item-tag">
              #{tag}
            </span>
          ))}
          {note.tags.length > 3 && <span className="notes-item-tag">+{note.tags.length - 3}</span>}
        </div>
      </div>
      {note.thumbnail && <img className="notes-thumb" src={note.thumbnail} alt="" loading="lazy" />}
    </Link>
  );
}
