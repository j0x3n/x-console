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
  Link2,
  ListChecks,
  NotebookPen,
  Notebook,
  Pin,
  Plus,
  Search,
  StickyNote,
  Trash2,
  X,
} from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import {
  useCreateNote,
  useDeleteNote,
  useNoteCounts,
  useNotes,
  useSetTagColor,
  useTags,
  type NoteKind,
  type NoteSummary,
} from "./api";
import { useFloatingNotes } from "./floating";
import MemoBoard from "./MemoBoard";
import { noteBgClass } from "./noteColors";
import { tagColor, TAG_COLORS } from "./tagColor";
import { useVaultStatus } from "../vault/api";
import NoteEditor from "./components/NoteEditor";
import PaneResizer from "./components/PaneResizer";
import {
  DATE_GROUP_LABELS,
  DEFAULT_PANES,
  PANE_KEY,
  PANE_LIMITS,
  dateGroup,
  noteTitle,
  readPanes,
  snippetParts,
  type DateGroup,
  type PaneWidths,
} from "./logic";
import PageActions from "../../components/layout/PageActions";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import { toast } from "../../hooks/useToast";
import { useKeepScroll } from "../../hooks/useKeepScroll";

type View = "all" | "pinned" | "memos" | "archived" | "tag" | "hidden";

export default function NotesPage() {
  const t = useT();
  const navigate = useNavigate();
  const { noteId } = useParams();
  const [search, setSearch] = useSearchParams();
  // B80：列表的滚动位置按筛选条件记（不含打开的是哪条）
  const listRef = useRef<HTMLDivElement>(null);
  useKeepScroll(listRef, `notes.list:${search.toString()}`);
  const q = search.get("q") ?? "";
  const tag = search.get("tag") ?? "";
  const archived = search.get("archived") === "1";
  const pinned = search.get("pinned") === "1";
  // B73：便签视图，右边是瀑布流
  const memosView = search.get("view") === "memos";
  const vault = useVaultStatus();
  const vaultUnlocked = vault.data?.unlocked ?? false;
  const [panes, setPanes] = useState<PaneWidths>(() => {
    try {
      return readPanes(localStorage);
    } catch {
      return DEFAULT_PANES;
    }
  });
  const panesRef = useRef(panes);
  panesRef.current = panes;
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
          : memosView
            ? "memos"
            : "all";
  const [input, setInput] = useState(q);
  const searchRef = useRef<HTMLInputElement>(null);
  const create = useCreateNote();
  const remove = useDeleteNote();
  // 选择模式（B23）：勾选多条后一起删除。
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const tags = useTags(hidden);
  const counts = useNoteCounts();
  const floatNote = useFloatingNotes((st) => st.open);
  // 全部、置顶、标签视图的列表只列笔记，便签在右边的瀑布流里（B73）。
  // 归档、隐藏和搜索时两种都列。
  const listKind: NoteKind | undefined =
    archived || hidden || q ? undefined : "note";
  const notes = useNotes({
    q,
    tag,
    archived: hidden ? false : archived,
    pinned: hidden ? false : pinned,
    hidden,
    kind: listKind,
  });
  const tagMemos = tag
    ? (tags.data?.find((tc) => tc.tag === tag)?.memoCount ?? 0)
    : 0;
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
        if (next === "memos") p.set("view", "memos");
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

  const stopSelecting = () => {
    setSelecting(false);
    setSelected(new Set());
  };
  const toggle = (noteId: number) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(noteId)) next.delete(noteId);
      else next.add(noteId);
      return next;
    });
  const afterDelete = (ids: number[]) => {
    if (id && ids.includes(id)) navigate(`/notes${query}`);
  };
  const deleteOne = async (n: NoteSummary) => {
    const name = noteTitle(n.title, n.excerpt) || t("Untitled note");
    if (
      !(await confirmAction({
        title: `${t("Delete")}“${name}”？`,
        description: t("This cannot be undone."),
      }))
    )
      return;
    remove.mutate(n.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        afterDelete([n.id]);
      },
    });
  };
  const deleteSelected = async () => {
    const ids = [...selected];
    if (
      !(await confirmAction({
        title: `${t("Delete")} ${ids.length} ${t("selected notes")}？`,
        description: t("This cannot be undone."),
      }))
    )
      return;
    let failed = 0;
    for (const noteId of ids) {
      try {
        await remove.mutateAsync(noteId);
      } catch {
        failed++;
      }
    }
    toast(
      failed
        ? {
            message: `${failed} ${t("notes could not be deleted")}`,
            tone: "error",
          }
        : t("Deleted"),
    );
    afterDelete(ids);
    stopSelecting();
  };

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
          : view === "memos"
            ? t("Memos")
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
      counts.data?.notes,
    ),
    navItem(
      "pinned",
      view === "pinned",
      () => setView("pinned"),
      <Pin size={15} />,
      t("Pinned notes"),
      counts.data?.pinned,
    ),
    navItem(
      "memos",
      view === "memos",
      () => setView("memos"),
      <StickyNote size={15} />,
      t("Memos"),
      counts.data?.memos,
    ),
    navItem(
      "archived",
      view === "archived",
      () => setView("archived"),
      <Archive size={15} />,
      t("Archived"),
      counts.data?.archived,
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

  const savePane = (key: keyof PaneWidths, width: number) => {
    const next = { ...panesRef.current, [key]: width };
    setPanes(next);
    try {
      localStorage.setItem(PANE_KEY, JSON.stringify(next));
    } catch {
      /* 存不了就只在这次生效 */
    }
  };
  const setPane = (key: keyof PaneWidths, width: number) =>
    setPanes((p) => ({ ...p, [key]: width }));

  return (
    <div
      className={`notes-layout ${id ? "has-note" : ""}${view === "memos" && !id ? " memos-view" : ""}`}
      style={
        {
          "--notes-nav-w": `${panes.nav}px`,
          "--notes-list-w": `${panes.list}px`,
        } as CSSProperties
      }
    >
      <PaneResizer
        className="notes-resizer-nav"
        width={panes.nav}
        {...PANE_LIMITS.nav}
        onChange={(w) => setPane("nav", w)}
        onDone={(w) => savePane("nav", w)}
        onReset={() => savePane("nav", DEFAULT_PANES.nav)}
      />
      <PaneResizer
        className="notes-resizer-list"
        width={panes.list}
        {...PANE_LIMITS.list}
        onChange={(w) => setPane("list", w)}
        onDone={(w) => savePane("list", w)}
        onReset={() => savePane("list", DEFAULT_PANES.list)}
      />
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
              {view === "memos"
                ? (counts.data?.memos ?? "")
                : items.length > 0
                  ? `${items.length}${notes.hasNextPage ? "+" : ""}`
                  : ""}
            </span>
          </div>
          {items.length > 0 &&
            view !== "memos" &&
            (selecting ? (
              <div className="notes-select-bar">
                <label className="xc-check">
                  <input
                    type="checkbox"
                    checked={selected.size === items.length}
                    onChange={(e) =>
                      setSelected(
                        e.target.checked
                          ? new Set(items.map((n) => n.id))
                          : new Set(),
                      )
                    }
                  />
                  <span>{t("Select all")}</span>
                </label>
                <button className="xc-btn ghost small" onClick={stopSelecting}>
                  {t("Cancel")}
                </button>
              </div>
            ) : (
              <button
                className="xc-btn ghost small"
                onClick={() => setSelecting(true)}
                title={t("Select notes")}
              >
                <ListChecks size={14} /> {t("Select")}
              </button>
            ))}
          <PageActions>
            {selecting ? (
              <button
                className="xc-btn danger small"
                disabled={selected.size === 0 || remove.isPending}
                onClick={deleteSelected}
                title={t("Delete selected")}
              >
                <Trash2 size={14} /> {t("Delete")} {selected.size} {t("notes")}
              </button>
            ) : (
              <button
                className="xc-btn primary small"
                onClick={newNote}
                disabled={create.isPending}
                title={t("New note")}
              >
                <Plus size={14} /> {t("New note")}
              </button>
            )}
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
        <div className="notes-list" role="list" ref={listRef}>
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
                    selecting={selecting}
                    checked={selected.has(n.id)}
                    onToggle={() => toggle(n.id)}
                    onDelete={() => deleteOne(n)}
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
          <NoteEditor
            id={id}
            backTo={`/notes${query}`}
            onPopOut={() => {
              floatNote(id);
              navigate(`/notes${query}`);
            }}
          />
        ) : view === "memos" ? (
          <MemoBoard hidden={hidden} />
        ) : tag && tagMemos > 0 ? (
          <MemoBoard tag={tag} hidden={hidden} />
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
  selecting,
  checked,
  onToggle,
  onDelete,
}: {
  note: NoteSummary;
  active: boolean;
  query: string;
  selecting: boolean;
  checked: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const tags = useTags();
  const colorOf = (tag: string) =>
    tagColor(tag, tags.data?.find((tc) => tc.tag === tag)?.color);
  const t = useT();
  const language = useLanguage();
  const [swiped, setSwiped] = useState(false);
  const touch = useRef<{ x: number; y: number } | null>(null);
  const title = noteTitle(note.title, note.excerpt) || t("Untitled note");
  const body = note.title.trim()
    ? note.excerpt
    : note.excerpt.slice(title.length).trim();
  const content = (
    <>
      {selecting && (
        <input
          type="checkbox"
          className="notes-item-check"
          checked={checked}
          aria-label={`${t("Select")} ${title}`}
          onChange={onToggle}
          onClick={(e) => e.stopPropagation()}
        />
      )}
      <div className="notes-item-text">
        <strong>
          {note.pinned && (
            <Pin size={11} className="notes-pin" aria-label={t("Pinned")} />
          )}
          {note.kind === "memo" && (
            <StickyNote
              size={11}
              className="notes-pin"
              aria-label={t("Memo")}
            />
          )}
          <span>{title}</span>
          {note.shared && (
            <Link2
              size={11}
              className="notes-item-shared"
              aria-label={t("Shared by link")}
            />
          )}
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
    </>
  );
  if (selecting)
    return (
      <div
        role="listitem"
        className={`notes-item selecting${checked ? " checked" : ""}`}
        onClick={onToggle}
      >
        {content}
      </div>
    );
  // 手机上左滑露出“删除”。
  return (
    <div
      role="listitem"
      className={`notes-item-row${swiped ? " swiped" : ""}`}
      onTouchStart={(e) => {
        touch.current = { x: e.touches[0].clientX, y: e.touches[0].clientY };
      }}
      onTouchMove={(e) => {
        const start = touch.current;
        if (!start) return;
        const dx = e.touches[0].clientX - start.x;
        const dy = e.touches[0].clientY - start.y;
        if (Math.abs(dx) > 40 && Math.abs(dx) > Math.abs(dy) * 2) {
          setSwiped(dx < 0);
          touch.current = null;
        }
      }}
    >
      <Link
        to={`/notes/${note.id}${query}`}
        className={`notes-item${active ? " active" : ""} ${noteBgClass(note.color)}`}
        aria-current={active ? "page" : undefined}
        onClick={(e) => {
          if (swiped) {
            e.preventDefault();
            setSwiped(false);
          }
        }}
      >
        {content}
      </Link>
      <MoreMenu
        className="notes-item-more"
        label={`${t("More")}：${title}`}
        title={title}
        items={[
          {
            key: "delete",
            label: t("Delete"),
            icon: <Trash2 size={14} />,
            danger: true,
            onSelect: onDelete,
          },
        ]}
      />
      <button
        className="notes-item-swipe-delete"
        tabIndex={swiped ? 0 : -1}
        aria-hidden={!swiped}
        onClick={() => {
          setSwiped(false);
          onDelete();
        }}
      >
        <Trash2 size={15} /> {t("Delete")}
      </button>
    </div>
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
