import {
  useEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
  type KeyboardEvent,
} from "react";
import { ChevronsLeftRight, ImagePlus, Plus, X } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  attachmentMarkdown,
  IMAGE_TYPES,
  MAX_IMAGE_BYTES,
  uploadFile,
} from "../../../components/markdown/upload";
import MoreMenu from "../../../components/ui/MoreMenu";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import {
  STATUS_LABELS,
  isNoopListMove,
  listColumn,
  planListMove,
  type Issue,
  type ListDropTarget,
  type ListMovePlan,
} from "../logic";
import type { Board, BoardList } from "../api";
import IssueCard from "./IssueCard";
import { StatusIcon } from "./Icons";

/** 列表颜色，名字对应 projects.css 里的 .list-color-* */
export const LIST_COLORS = [
  "",
  "red",
  "orange",
  "yellow",
  "green",
  "blue",
  "purple",
] as const;

export interface ListActions {
  /** description 是快速添加时贴进来的图片（B55） */
  quickAdd: (
    listId: number,
    title: string,
    description?: string,
  ) => Promise<unknown>;
  addList: (name: string) => Promise<unknown>;
  settings: (list: BoardList) => void;
  toggleCollapse: (list: BoardList) => void;
  moveAll: (list: BoardList) => void;
  archiveCards: (list: BoardList) => void;
  archiveList: (list: BoardList) => void;
  deleteList: (list: BoardList) => void;
}

/** 一行输入：回车提交，Esc 关闭。提交后清空，方便连续输入。 */
function InlineInput({
  placeholder,
  onSubmit,
  onClose,
  multiline,
}: {
  placeholder: string;
  onSubmit: (text: string) => Promise<unknown>;
  onClose: () => void;
  multiline?: boolean;
}) {
  const t = useT();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const ref = useRef<HTMLTextAreaElement & HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  const submit = async () => {
    const value = text.trim();
    if (!value || busy) return;
    setBusy(true);
    try {
      await onSubmit(value);
      setText("");
    } finally {
      setBusy(false);
      ref.current?.focus();
    }
  };
  const props = {
    ref,
    value: text,
    placeholder,
    className: "xc-input projects-inline-input",
    onChange: (e: { target: { value: string } }) => setText(e.target.value),
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
        e.preventDefault();
        void submit();
      }
      if (e.key === "Escape") onClose();
    },
  };
  return (
    <div className="projects-inline">
      {multiline ? <textarea rows={2} {...props} /> : <input {...props} />}
      <div className="projects-inline-actions">
        <button
          className="xc-btn primary small"
          disabled={!text.trim() || busy}
          onClick={() => void submit()}
        >
          {t("Add")}
        </button>
        <button className="xc-btn ghost small" onClick={onClose}>
          {t("Cancel")}
        </button>
      </div>
    </div>
  );
}

interface PendingImage {
  id: string;
  name: string;
  preview: string;
  markdown?: string;
  failed?: boolean;
}

/**
 * 快速添加卡片（B55）：一行标题，可以粘贴、拖入或选择图片。
 * 图片先上传，加卡片时写进描述，卡片上显示第一张做封面。
 */
function CardInput({
  onSubmit,
  onClose,
}: {
  onSubmit: (title: string, description?: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const t = useT();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [images, setImages] = useState<PendingImage[]>([]);
  const ref = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  const imagesRef = useRef(images);
  imagesRef.current = images;
  useEffect(
    () => () =>
      imagesRef.current.forEach((i) => URL.revokeObjectURL(i.preview)),
    [],
  );

  const uploading = images.some((i) => !i.markdown && !i.failed);
  const add = (files: File[]) => {
    const ok = files.filter((f) => {
      if (!IMAGE_TYPES.includes(f.type)) {
        toast({
          message: `${f.name}: ${t("Only images can be added here")}`,
          tone: "error",
        });
        return false;
      }
      if (f.size > MAX_IMAGE_BYTES) {
        toast({
          message: `${f.name}: ${t("Image is larger than 20 MB")}`,
          tone: "error",
        });
        return false;
      }
      return true;
    });
    for (const file of ok) {
      const item: PendingImage = {
        id: Math.random().toString(36).slice(2, 9),
        name: file.name || "image.png",
        preview: URL.createObjectURL(file),
      };
      setImages((prev) => [...prev, item]);
      uploadFile("projects", file).then(
        (f) =>
          setImages((prev) =>
            prev.map((x) =>
              x.id === item.id ? { ...x, markdown: attachmentMarkdown(f) } : x,
            ),
          ),
        (err) => {
          toast({
            message: `${item.name}: ${errorMessage(err)}`,
            tone: "error",
          });
          setImages((prev) =>
            prev.map((x) => (x.id === item.id ? { ...x, failed: true } : x)),
          );
        },
      );
    }
  };
  const remove = (id: string) =>
    setImages((prev) => {
      const gone = prev.find((x) => x.id === id);
      if (gone) URL.revokeObjectURL(gone.preview);
      return prev.filter((x) => x.id !== id);
    });

  const submit = async () => {
    const done = images.filter((i) => i.markdown);
    // 只有图片没写标题时，用第一张图的文件名当标题
    const value =
      text.trim() || (done[0]?.name.replace(/\.[a-z0-9]+$/i, "") ?? "");
    if (!value || busy || uploading) return;
    setBusy(true);
    try {
      await onSubmit(
        value,
        done.length ? done.map((i) => i.markdown).join("\n\n") : undefined,
      );
      setText("");
      images.forEach((i) => URL.revokeObjectURL(i.preview));
      setImages([]);
    } finally {
      setBusy(false);
      ref.current?.focus();
    }
  };

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(e.clipboardData.files);
    // 从 Excel、Word 复制时同时有文字和图片，这时按文字粘贴
    if (!files.length || e.clipboardData.getData("text/plain")) return;
    e.preventDefault();
    add(files);
  };

  return (
    <div
      className="projects-inline"
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes("Files")) e.preventDefault();
      }}
      onDrop={(e) => {
        const files = Array.from(e.dataTransfer.files);
        if (!files.length) return;
        e.preventDefault();
        e.stopPropagation();
        add(files);
      }}
    >
      <textarea
        ref={ref}
        rows={2}
        value={text}
        placeholder={t("Card title. Paste images here.")}
        className="xc-input projects-inline-input"
        onChange={(e) => setText(e.target.value)}
        onPaste={onPaste}
        onKeyDown={(e: KeyboardEvent) => {
          if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            void submit();
          }
          if (e.key === "Escape") onClose();
        }}
      />
      {images.length > 0 && (
        <div className="projects-inline-images">
          {images.map((i) => (
            <span
              key={i.id}
              className={`projects-inline-image${i.markdown ? "" : i.failed ? " failed" : " loading"}`}
              title={i.name}
            >
              <img src={i.preview} alt="" />
              <button
                type="button"
                aria-label={`${t("Remove")} ${i.name}`}
                onClick={() => remove(i.id)}
              >
                <X size={11} />
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="projects-inline-actions">
        <button
          className="xc-btn primary small"
          disabled={
            (!text.trim() && !images.some((i) => i.markdown)) ||
            busy ||
            uploading
          }
          onClick={() => void submit()}
        >
          {uploading ? t("Uploading…") : t("Add")}
        </button>
        <button className="xc-btn ghost small" onClick={onClose}>
          {t("Cancel")}
        </button>
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Add image")}
          aria-label={t("Add image")}
          onClick={() => fileRef.current?.click()}
        >
          <ImagePlus size={14} />
        </button>
        <input
          ref={fileRef}
          type="file"
          accept={IMAGE_TYPES.join(",")}
          multiple
          hidden
          onChange={(e) => {
            const files = Array.from(e.target.files ?? []);
            e.target.value = "";
            add(files);
          }}
        />
      </div>
    </div>
  );
}

/**
 * 横向滚动的看板（B55）：滚动条平时隐藏，两边还能滚时有渐隐；
 * 在空白处按住拖动、或者用滚轮都能左右滚。
 */
function useBoardScroll() {
  const ref = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ left: false, right: false });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () =>
      setEdges({
        left: el.scrollLeft > 4,
        right: el.scrollLeft + el.clientWidth < el.scrollWidth - 4,
      });
    update();
    const onWheel = (e: WheelEvent) => {
      if (el.scrollWidth <= el.clientWidth || e.shiftKey) return;
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
      // 列表自己能上下滚时交给列表
      const list = (e.target as HTMLElement).closest?.(".projects-lane-list");
      if (list && list.scrollHeight > list.clientHeight) return;
      e.preventDefault();
      el.scrollLeft += e.deltaY;
    };
    let drag: { x: number; left: number; id: number } | null = null;
    const onDown = (e: PointerEvent) => {
      const target = e.target as HTMLElement;
      if (e.pointerType !== "mouse" || e.button !== 0) return;
      if (target !== el && !target.classList.contains("projects-lane-list"))
        return;
      drag = { x: e.clientX, left: el.scrollLeft, id: e.pointerId };
      el.setPointerCapture(e.pointerId);
      el.classList.add("grabbing");
    };
    const onMove = (e: PointerEvent) => {
      if (drag && e.pointerId === drag.id)
        el.scrollLeft = drag.left - (e.clientX - drag.x);
    };
    const onUp = (e: PointerEvent) => {
      if (!drag || e.pointerId !== drag.id) return;
      drag = null;
      el.classList.remove("grabbing");
    };
    // 测试环境和很旧的浏览器没有 ResizeObserver，这时只在滚动时更新
    const ro =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    ro?.observe(el);
    el.addEventListener("scroll", update, { passive: true });
    el.addEventListener("wheel", onWheel, { passive: false });
    el.addEventListener("pointerdown", onDown);
    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerup", onUp);
    el.addEventListener("pointercancel", onUp);
    return () => {
      ro?.disconnect();
      el.removeEventListener("scroll", update);
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("pointerdown", onDown);
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerup", onUp);
      el.removeEventListener("pointercancel", onUp);
    };
  }, []);
  const cls = `${edges.left ? " fade-left" : ""}${edges.right ? " fade-right" : ""}`;
  return { ref, cls };
}

/**
 * 拖动时跟着鼠标的卡片（B55）。浏览器默认截的图会带上列表背景、
 * 还会被截掉一块，这里复制一张干净的卡片当拖动图。
 */
function setCardDragImage(e: DragEvent<HTMLElement>) {
  const el = e.currentTarget;
  const rect = el.getBoundingClientRect();
  if (!e.dataTransfer.setDragImage) return rect.height;
  const ghost = el.cloneNode(true) as HTMLElement;
  ghost.classList.remove("selected", "dragging");
  ghost.classList.add("projects-drag-ghost");
  ghost.style.width = `${rect.width}px`;
  document.body.appendChild(ghost);
  e.dataTransfer.setDragImage(
    ghost,
    e.clientX - rect.left,
    e.clientY - rect.top,
  );
  setTimeout(() => ghost.remove(), 0);
  return rect.height;
}

/** 看板（B46）：一个看板的所有列表，卡片可以拖到任意列表。 */
export default function ListBoard({
  board,
  issues,
  selectedKey,
  onSelect,
  onOpen,
  onMove,
  actions,
  readOnly,
  locked,
}: {
  board: Board;
  /** 这个看板上（筛选后）的卡片 */
  issues: Issue[];
  selectedKey: string | null;
  onSelect: (key: string) => void;
  onOpen: (key: string) => void;
  onMove: (key: string, plan: ListMovePlan) => void;
  actions: ListActions;
  readOnly?: boolean;
  /** 锁定看板结构（B55）：不能加列表，列表菜单里只留卡片相关的操作 */
  locked?: boolean;
}) {
  const t = useT();
  const [dragKey, setDragKey] = useState<string | null>(null);
  const [dragHeight, setDragHeight] = useState(40);
  const [drop, setDrop] = useState<ListDropTarget | null>(null);
  const [adding, setAdding] = useState<number | null>(null);
  const [addingList, setAddingList] = useState(false);
  const scroll = useBoardScroll();

  const reset = () => {
    setDragKey(null);
    setDrop(null);
  };
  const statusOf = (listId: number) =>
    board.lists.find((l) => l.id === listId)?.status;
  const finish = (e: DragEvent) => {
    e.preventDefault();
    const key = dragKey ?? e.dataTransfer.getData("text/plain");
    if (key && drop) {
      const plan = planListMove(issues, key, drop, statusOf(drop.listId));
      if (!isNoopListMove(issues, key, plan)) onMove(key, plan);
    }
    reset();
  };

  return (
    <div
      ref={scroll.ref}
      className={`projects-board lists${scroll.cls}`}
      onDragEnd={reset}
    >
      {board.lists.map((list) => {
        const cards = listColumn(issues, list.id);
        const others = cards.filter((i) => i.key !== dragKey);
        const dropIndex = drop?.listId === list.id ? drop.index : -1;
        const over = list.wipLimit > 0 && list.cardCount > list.wipLimit;
        const line = (i: number) =>
          dropIndex === i ? (
            <div
              className="projects-drop-slot"
              key={`slot-${i}`}
              style={{ height: dragHeight }}
            />
          ) : null;
        if (list.collapsed)
          return (
            <button
              key={list.id}
              className={`projects-lane collapsed list-color-${list.color || "none"}`}
              title={t("Expand list")}
              onClick={() => actions.toggleCollapse(list)}
              onDragOver={(e) => {
                if (!dragKey) return;
                e.preventDefault();
                setDrop({ listId: list.id, index: others.length });
              }}
              onDrop={finish}
            >
              <ChevronsLeftRight size={14} />
              <span>{list.name}</span>
              <em>{list.cardCount}</em>
            </button>
          );
        const menu = [
          ...(locked
            ? []
            : [
                {
                  key: "settings",
                  label: t("List settings"),
                  onSelect: () => actions.settings(list),
                },
              ]),
          {
            key: "collapse",
            label: t("Collapse list"),
            onSelect: () => actions.toggleCollapse(list),
          },
          {
            key: "move",
            label: t("Move all cards…"),
            onSelect: () => actions.moveAll(list),
          },
          {
            key: "archive-cards",
            label: t("Archive all cards"),
            onSelect: () => actions.archiveCards(list),
          },
          ...(locked
            ? []
            : [
                {
                  key: "archive",
                  label: t("Archive list"),
                  onSelect: () => actions.archiveList(list),
                },
                {
                  key: "delete",
                  label: t("Delete list"),
                  danger: true,
                  onSelect: () => actions.deleteList(list),
                },
              ]),
        ];
        return (
          <section
            key={list.id}
            className={`projects-lane list-color-${list.color || "none"} ${dropIndex >= 0 ? "drag-over" : ""}`}
            data-list-id={list.id}
            onDragOver={(e) => {
              if (!dragKey) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
              if (
                e.target === e.currentTarget ||
                (e.target as HTMLElement).classList.contains(
                  "projects-lane-list",
                )
              )
                setDrop({ listId: list.id, index: others.length });
            }}
            onDrop={finish}
          >
            <header className="projects-lane-head">
              {list.status && <StatusIcon status={list.status} size={14} />}
              <h2
                title={
                  list.status
                    ? `${t("Cards here become")} ${t(STATUS_LABELS[list.status])}`
                    : undefined
                }
              >
                {list.name}
              </h2>
              <em className={over ? "over" : ""}>
                {list.wipLimit > 0
                  ? `${list.cardCount}/${list.wipLimit}`
                  : list.cardCount}
              </em>
              <span className="xc-spacer" />
              {!readOnly && (
                <MoreMenu
                  label={`${t("More")}：${list.name}`}
                  title={list.name}
                  items={menu}
                />
              )}
            </header>
            <div className="projects-lane-list">
              {cards.map((issue) => {
                const index = others.findIndex((o) => o.key === issue.key);
                return [
                  index >= 0 ? line(index) : null,
                  <IssueCard
                    key={issue.key}
                    issue={issue}
                    selected={issue.key === selectedKey}
                    dragging={issue.key === dragKey}
                    onClick={() => {
                      onSelect(issue.key);
                      onOpen(issue.key);
                    }}
                    onDragStart={(e) => {
                      e.dataTransfer.setData("text/plain", issue.key);
                      e.dataTransfer.effectAllowed = "move";
                      setDragHeight(setCardDragImage(e));
                      setDragKey(issue.key);
                      onSelect(issue.key);
                    }}
                    onDragOver={(e) => {
                      if (!dragKey) return;
                      e.preventDefault();
                      e.stopPropagation();
                      if (issue.key === dragKey) return;
                      const rect = e.currentTarget.getBoundingClientRect();
                      const before = e.clientY < rect.top + rect.height / 2;
                      setDrop({
                        listId: list.id,
                        index: index + (before ? 0 : 1),
                      });
                    }}
                  />,
                ];
              })}
              {line(others.length)}
            </div>
            {!readOnly &&
              (adding === list.id ? (
                <CardInput
                  onSubmit={(title, description) =>
                    actions.quickAdd(list.id, title, description)
                  }
                  onClose={() => setAdding(null)}
                />
              ) : (
                <button
                  className="projects-add-card"
                  onClick={() => setAdding(list.id)}
                >
                  <Plus size={14} /> {t("Add card")}
                </button>
              ))}
          </section>
        );
      })}
      {!readOnly && !locked && (
        <section className="projects-lane add-list">
          {addingList ? (
            <InlineInput
              placeholder={t("List name")}
              onSubmit={actions.addList}
              onClose={() => setAddingList(false)}
            />
          ) : (
            <button
              className="projects-add-card"
              onClick={() => setAddingList(true)}
            >
              <Plus size={14} /> {t("Add list")}
            </button>
          )}
        </section>
      )}
    </div>
  );
}

/** 删除列表前确认。列表里还有卡片时后端会拒绝，这里先说清楚。 */
export async function confirmDeleteList(
  list: BoardList,
  t: (s: string) => string,
) {
  return confirmAction({
    title: `${t("Delete list")}“${list.name}”？`,
    description: t(
      "Only an empty list can be deleted. Move or archive its cards first.",
    ),
    confirmLabel: t("Delete"),
  });
}
