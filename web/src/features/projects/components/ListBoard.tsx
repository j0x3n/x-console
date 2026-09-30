import {
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
} from "react";
import { ChevronsLeftRight, Plus } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
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
  quickAdd: (listId: number, title: string) => Promise<unknown>;
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
}) {
  const t = useT();
  const [dragKey, setDragKey] = useState<string | null>(null);
  const [drop, setDrop] = useState<ListDropTarget | null>(null);
  const [adding, setAdding] = useState<number | null>(null);
  const [addingList, setAddingList] = useState(false);

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
    <div className="projects-board lists" onDragEnd={reset}>
      {board.lists.map((list) => {
        const cards = listColumn(issues, list.id);
        const others = cards.filter((i) => i.key !== dragKey);
        const dropIndex = drop?.listId === list.id ? drop.index : -1;
        const over = list.wipLimit > 0 && list.cardCount > list.wipLimit;
        const line = (i: number) =>
          dropIndex === i ? (
            <div className="projects-drop-line" key={`line-${i}`} />
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
                  items={[
                    {
                      key: "settings",
                      label: t("List settings"),
                      onSelect: () => actions.settings(list),
                    },
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
                  ]}
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
                <InlineInput
                  multiline
                  placeholder={t("Card title, Enter to add")}
                  onSubmit={(title) => actions.quickAdd(list.id, title)}
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
      {!readOnly && (
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
