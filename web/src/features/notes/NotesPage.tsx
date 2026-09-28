import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import {
  Archive,
  EyeOff,
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
import {
  useCreateNote,
  useNotes,
  useSetTagColor,
  useTags,
  type NoteSummary,
} from "./api";
import { tagColor, TAG_COLORS } from "./tagColor";
import { useVaultStatus } from "../vault/api";
import NoteEditor from "./components/NoteEditor";
import {
  DATE_GROUP_LABELS,
  dateGroup,
  noteTitle,
  snippetParts,
  type DateGroup,
} from "./logic";
import PageActions from "../../components/layout/PageActions";

type View = "all" | "pinned" | "archived" | "tag" | "hidden";

export default function NotesPage() {
  const t = useT();
  const navigate = useNavigate();
  const { noteId } = useParams();
  const [search, setSearch] = useSearchParams();
  const q = search.get("q") ?? "";
  const tag = search.get("tag") ?? "";
  const archived = search.get("archived") === "1";
  const pinned = search.get("pinned") === "1";
  const vault = useVaultStatus();
  const vaultUnlocked = vault.data?.unlocked ?? false;
  // 隐藏分类只在解锁后出现。地址栏带着 hidden=1 但已锁定时，当作全部笔记。
  const hidden = search.get("hidden") === "1" && vaultUnlocked;
  const view: View = hidden
    ? "hidden"
    : tag
      ? "tag"
      : archived
        ? "archived"
        : pinned
          ? "pinned"
          : "all";
  const [input, setInput] = useState(q);
  const searchRef = useRef<HTMLInputElement>(null);
  const create = useCreateNote();
  const tags = useTags(hidden);
  const notes = useNotes({
    q,
    tag,
    archived: hidden ? false : archived,
    pinned: hidden ? false : pinned,
    hidden,
  });
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
        // 隐藏空间里点标签，还留在隐藏空间
        if (next === "hidden" || (next === "tag" && hidden))
          p.set("hidden", "1");
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
      hidden
        ? { hidden: true, ...(tag ? { tags: [tag] } : {}) }
        : { tags: tag ? [tag] : [], pinned: pinned || undefined },
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
  const viewTitle = tag
    ? `#${tag}`
    : view === "pinned"
      ? t("Pinned notes")
      : view === "archived"
        ? t("Archived")
        : view === "hidden"
          ? t("Hidden notes")
          : t("All notes");

  const navItem = (
    key: string,
    active: boolean,
    onClick: () => void,
    icon: ReactNode,
    label: string,
    count?: number,
  ) => (
    <button
      key={key}
      className={active ? "active" : ""}
      aria-pressed={active}
      onClick={onClick}
    >
      {icon}
      <span>{label}</span>
      {count != null && <small>{count}</small>}
    </button>
  );

  const views = [
    navItem(
      "all",
      view === "all",
      () => setView("all"),
      <Notebook size={15} />,
      t("All notes"),
    ),
    navItem(
      "pinned",
      view === "pinned",
      () => setView("pinned"),
      <Pin size={15} />,
      t("Pinned notes"),
    ),
    navItem(
      "archived",
      view === "archived",
      () => setView("archived"),
      <Archive size={15} />,
      t("Archived"),
    ),
    ...(vaultUnlocked
      ? [
          navItem(
            "hidden",
            view === "hidden",
            () => setView("hidden"),
            <EyeOff size={15} />,
            t("Hidden notes"),
          ),
        ]
      : []),
  ];
  const tagItems =
    tags.data?.map((tc) =>
      navItem(
        `tag-${tc.tag}`,
        tag === tc.tag,
        () =>
          setView(tag === tc.tag ? (hidden ? "hidden" : "all") : "tag", tc.tag),
        <i
          className="notes-tag-dot"
          style={{ background: tagColor(tc.tag, tc.color) }}
        />,
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
            {tag && (
              <TagColorPicker
                tag={tag}
                color={tags.data?.find((tc) => tc.tag === tag)?.color}
              />
            )}
            <h1>{viewTitle}</h1>
            <span>
              {items.length > 0
                ? `${items.length}${notes.hasNextPage ? "+" : ""}`
                : ""}
            </span>
          </div>
          <PageActions>
            <button
              className="xc-btn primary small"
              onClick={newNote}
              disabled={create.isPending}
              title={t("New note")}
            >
              <Plus size={14} /> {t("New note")}
            </button>
          </PageActions>
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
                    : view === "hidden"
                      ? t("No hidden notes")
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
                  <NoteItem
                    key={n.id}
                    note={n}
                    active={n.id === id}
                    query={query}
                  />
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
function groupNotes(
  items: NoteSummary[],
  searching: boolean,
  view: View,
): Group[] {
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
    if (n.pinned && view !== "pinned" && view !== "archived")
      add("pinned", "Pinned notes", n);
    else {
      const g: DateGroup = dateGroup(n.updatedAt, now);
      add(g, DATE_GROUP_LABELS[g], n);
    }
  }
  return groups;
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
  const tags = useTags();
  const colorOf = (tag: string) =>
    tagColor(tag, tags.data?.find((tc) => tc.tag === tag)?.color);
  const t = useT();
  const language = useLanguage();
  const title = noteTitle(note.title, note.excerpt) || t("Untitled note");
  const body = note.title.trim()
    ? note.excerpt
    : note.excerpt.slice(title.length).trim();
  return (
    <Link
      to={`/notes/${note.id}${query}`}
      role="listitem"
      className={`notes-item${active ? " active" : ""}`}
      aria-current={active ? "page" : undefined}
    >
      <div className="notes-item-text">
        <strong>
          {note.pinned && (
            <Pin size={11} className="notes-pin" aria-label={t("Pinned")} />
          )}
          <span>{title}</span>
        </strong>
        <p>
          {note.snippet
            ? snippetParts(note.snippet).map((part, i) =>
                part.hit ? (
                  <mark key={i}>{part.text}</mark>
                ) : (
                  <span key={i}>{part.text}</span>
                ),
              )
            : body || (
                <span className="notes-item-empty">{t("No more text")}</span>
              )}
        </p>
        <div className="notes-item-foot">
          <time dateTime={note.updatedAt}>
            {relativeTime(note.updatedAt, language)}
          </time>
          {note.tags.slice(0, 3).map((tag) => (
            <span
              key={tag}
              className="notes-item-tag"
              style={{ "--tag": colorOf(tag) } as CSSProperties}
            >
              #{tag}
            </span>
          ))}
          {note.tags.length > 3 && (
            <span className="notes-item-tag">+{note.tags.length - 3}</span>
          )}
        </div>
      </div>
      {note.thumbnail && (
        <img
          className="notes-thumb"
          src={note.thumbnail}
          alt=""
          loading="lazy"
        />
      )}
    </Link>
  );
}

/** 标签的颜色：点小圆点弹出一排可选的颜色。 */
function TagColorPicker({ tag, color }: { tag: string; color?: string }) {
  const t = useT();
  const save = useSetTagColor();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);
  const current = tagColor(tag, color);
  const pick = (c: string) => {
    save.mutate({ tag, color: c });
    setOpen(false);
  };
  return (
    <div className="notes-tag-color" ref={ref}>
      <button
        className="notes-tag-color-btn"
        aria-label={t("Tag color")}
        title={t("Tag color")}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <i className="notes-tag-dot" style={{ background: current }} />
      </button>
      {open && (
        <div className="notes-tag-color-menu" role="menu">
          {TAG_COLORS.map((c) => (
            <button
              key={c}
              role="menuitemradio"
              aria-checked={c === current}
              aria-label={c}
              className={c === current ? "on" : ""}
              style={{ background: c }}
              onClick={() => pick(c)}
            />
          ))}
          {color && (
            <button
              role="menuitem"
              className="notes-tag-color-reset"
              onClick={() => pick("")}
            >
              {t("Default color")}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
