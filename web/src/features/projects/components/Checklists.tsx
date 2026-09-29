import { Fragment, useState, type DragEvent } from "react";
import {
  ArrowUpRight,
  Eye,
  EyeOff,
  GripVertical,
  Plus,
  Trash2,
} from "lucide-react";
import { isNotLive } from "../../../api/client";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useChecklistMutations, useChecklists } from "../api";
import {
  checklistProgress,
  isNoopItemMove,
  planItemMove,
  sortItems,
  type Checklist,
  type ChecklistItem,
} from "../logic";

/** Issue 详情里的检查清单（B36）：勾选、编辑、拖动排序、转成 Issue。 */
export default function Checklists({
  issueKey,
  adding,
  onAddingChange,
}: {
  issueKey: string;
  /** 正在新建清单（Issue 的“更多”菜单也能打开） */
  adding: boolean;
  onAddingChange: (adding: boolean) => void;
}) {
  const t = useT();
  const lists = useChecklists(issueKey);
  const ops = useChecklistMutations(issueKey);
  const [title, setTitle] = useState("");

  if (lists.isError && isNotLive(lists.error))
    return (
      <section className="projects-section">
        <header>
          <h2>{t("Checklists")}</h2>
        </header>
        <p className="xc-muted projects-section-empty">
          {t("Checklists are not live yet.")}
        </p>
      </section>
    );

  const data = Array.isArray(lists.data) ? lists.data : [];
  const progress = checklistProgress(data);
  const create = () => {
    const name = title.trim() || t("Checklists");
    ops.createList.mutate(name, {
      onSuccess: () => {
        setTitle("");
        onAddingChange(false);
      },
    });
  };

  return (
    <section className="projects-section projects-checklists">
      <header>
        <h2>
          {t("Checklists")}
          {progress.total > 0 && (
            <span className="projects-checklist-count">
              {progress.done}/{progress.total}
            </span>
          )}
        </h2>
        <button
          className="xc-btn ghost small"
          onClick={() => onAddingChange(!adding)}
        >
          <Plus size={14} /> {t("Add checklist")}
        </button>
      </header>
      {lists.isError && (
        <p className="xc-error-text">{t("Something went wrong")}</p>
      )}
      {data.length === 0 && !adding && lists.isSuccess && (
        <p className="xc-muted projects-section-empty">
          {t("Split the work into small steps and tick them off.")}
        </p>
      )}
      {data.map((list) => (
        <ChecklistBlock key={list.id} list={list} ops={ops} />
      ))}
      {adding && (
        <form
          className="projects-inline-form"
          onSubmit={(e) => {
            e.preventDefault();
            create();
          }}
        >
          <input
            className="xc-input"
            value={title}
            autoFocus
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                e.stopPropagation();
                onAddingChange(false);
              }
            }}
            placeholder={t("Checklists")}
            aria-label={t("Checklist title")}
          />
          <button
            className="xc-btn small primary"
            disabled={ops.createList.isPending}
          >
            {t("Add")}
          </button>
        </form>
      )}
    </section>
  );
}

type Ops = ReturnType<typeof useChecklistMutations>;

function ChecklistBlock({ list, ops }: { list: Checklist; ops: Ops }) {
  const t = useT();
  const [hideDone, setHideDone] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(list.title);
  const [editing, setEditing] = useState<number | null>(null);
  // 新条目的输入框放在哪一条后面；undefined 表示放在最后。
  const [insertAfter, setInsertAfter] = useState<number | undefined>();
  const [newText, setNewText] = useState("");
  const [dragId, setDragId] = useState<number | null>(null);
  const [dropIndex, setDropIndex] = useState<number | null>(null);

  const items = sortItems(list.items);
  const done = items.filter((i) => i.done).length;
  const shown = hideDone ? items.filter((i) => !i.done) : items;
  const pct = items.length ? Math.round((done / items.length) * 100) : 0;

  const rename = () => {
    setRenaming(false);
    const title = name.trim();
    if (title && title !== list.title)
      ops.renameList.mutate({ id: list.id, title });
    else setName(list.title);
  };

  const add = () => {
    const text = newText.trim();
    if (!text) return;
    ops.addItem.mutate(
      { checklistId: list.id, text, afterId: insertAfter },
      {
        onSuccess: (item) => {
          setNewText("");
          // 接着在刚加的那条后面输入下一条。
          if (insertAfter !== undefined) setInsertAfter(item.id);
        },
      },
    );
  };

  const drop = (e: DragEvent) => {
    e.preventDefault();
    if (dragId !== null && dropIndex !== null) {
      const plan = planItemMove(items, dragId, dropIndex);
      if (!isNoopItemMove(items, dragId, plan))
        ops.updateItem.mutate({ id: dragId, body: plan });
    }
    setDragId(null);
    setDropIndex(null);
  };

  const newItemForm = (
    <li className="projects-checklist-new">
      <input
        className="xc-input"
        value={newText}
        autoFocus={insertAfter !== undefined}
        onChange={(e) => setNewText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            add();
          }
          if (e.key === "Escape") {
            e.stopPropagation();
            setNewText("");
            setInsertAfter(undefined);
          }
        }}
        onBlur={() => {
          if (!newText.trim()) setInsertAfter(undefined);
        }}
        placeholder={t("Add an item")}
        aria-label={`${t("Add an item")} · ${list.title}`}
      />
    </li>
  );

  return (
    <div className="projects-checklist">
      <div className="projects-checklist-head">
        {renaming ? (
          <input
            className="xc-input"
            value={name}
            autoFocus
            aria-label={t("Checklist title")}
            onChange={(e) => setName(e.target.value)}
            onBlur={rename}
            onKeyDown={(e) => {
              if (e.key === "Enter") rename();
              if (e.key === "Escape") {
                e.stopPropagation();
                setName(list.title);
                setRenaming(false);
              }
            }}
          />
        ) : (
          <h3 onClick={() => setRenaming(true)} title={t("Click to rename")}>
            {list.title}
          </h3>
        )}
        <span className="projects-checklist-count">
          {done}/{items.length}
        </span>
        <span className="xc-spacer" />
        {done > 0 && (
          <button
            className="xc-btn ghost small"
            aria-pressed={hideDone}
            onClick={() => setHideDone((v) => !v)}
          >
            {hideDone ? <Eye size={13} /> : <EyeOff size={13} />}
            {hideDone ? t("Show done items") : t("Hide done items")}
          </button>
        )}
        <button
          className="xc-btn ghost small"
          aria-label={`${t("Delete checklist")} ${list.title}`}
          onClick={async () =>
            (await confirmAction({
              title: `${t("Delete checklist")}“${list.title}”？`,
              description: t("All its items are deleted too."),
            })) && ops.deleteList.mutate(list.id)
          }
        >
          <Trash2 size={13} />
        </button>
      </div>
      {items.length > 0 && (
        <div
          className={`projects-checklist-bar${pct === 100 ? " complete" : ""}`}
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={list.title}
        >
          <i style={{ width: `${pct}%` }} />
        </div>
      )}
      <ul
        className="projects-checklist-items"
        onDragOver={(e) => {
          if (dragId === null) return;
          e.preventDefault();
          if (e.target === e.currentTarget)
            setDropIndex(items.filter((i) => i.id !== dragId).length);
        }}
        onDrop={drop}
        onDragEnd={() => {
          setDragId(null);
          setDropIndex(null);
        }}
      >
        {shown.map((item) => {
          const index = items
            .filter((i) => i.id !== dragId)
            .findIndex((i) => i.id === item.id);
          return [
            dropIndex === index && index >= 0 ? (
              <li key={`line-${item.id}`} className="projects-drop-line" />
            ) : null,
            <ItemRow
              key={item.id}
              item={item}
              editing={editing === item.id}
              dragging={dragId === item.id}
              onEdit={(on) => setEditing(on ? item.id : null)}
              onToggle={() =>
                ops.updateItem.mutate({
                  id: item.id,
                  body: { done: !item.done },
                })
              }
              onSave={(text, next) => {
                setEditing(null);
                if (text && text !== item.text)
                  ops.updateItem.mutate({ id: item.id, body: { text } });
                if (next) setInsertAfter(item.id);
              }}
              onDelete={() => ops.deleteItem.mutate(item.id)}
              onConvert={async () => {
                if (
                  !(await confirmAction({
                    title: t("Turn this item into an issue?"),
                    description: item.text,
                    confirmLabel: t("Convert to issue"),
                    danger: false,
                  }))
                )
                  return;
                ops.convertItem.mutate(item.id, {
                  onSuccess: (issue) =>
                    toast({
                      message: `${t("Created")} ${issue.key}`,
                      subtitle: issue.title,
                    }),
                });
              }}
              onDragStart={() => setDragId(item.id)}
              onDragOver={(before) => setDropIndex(before ? index : index + 1)}
            />,
            insertAfter === item.id ? (
              <Fragment key={`new-${item.id}`}>{newItemForm}</Fragment>
            ) : null,
          ];
        })}
        {dropIndex !== null &&
          dropIndex === items.filter((i) => i.id !== dragId).length && (
            <li className="projects-drop-line" />
          )}
        {insertAfter === undefined && newItemForm}
      </ul>
      {hideDone && done > 0 && (
        <p className="xc-muted projects-section-empty">
          {done} {t("done items hidden")}
        </p>
      )}
    </div>
  );
}

function ItemRow({
  item,
  editing,
  dragging,
  onEdit,
  onToggle,
  onSave,
  onDelete,
  onConvert,
  onDragStart,
  onDragOver,
}: {
  item: ChecklistItem;
  editing: boolean;
  dragging: boolean;
  onEdit: (on: boolean) => void;
  onToggle: () => void;
  /** next 为 true 表示按了回车，接着新建下一条 */
  onSave: (text: string, next: boolean) => void;
  onDelete: () => void;
  onConvert: () => void;
  onDragStart: () => void;
  onDragOver: (before: boolean) => void;
}) {
  const t = useT();
  const [text, setText] = useState(item.text);
  return (
    <li
      className={`projects-checklist-item${item.done ? " done" : ""}${dragging ? " dragging" : ""}`}
      draggable={!editing}
      onDragStart={(e) => {
        e.dataTransfer.setData("text/plain", String(item.id));
        e.dataTransfer.effectAllowed = "move";
        onDragStart();
      }}
      onDragOver={(e) => {
        e.preventDefault();
        e.stopPropagation();
        const rect = e.currentTarget.getBoundingClientRect();
        onDragOver(e.clientY < rect.top + rect.height / 2);
      }}
    >
      <GripVertical size={13} className="projects-checklist-grip" />
      <input
        type="checkbox"
        checked={item.done}
        onChange={onToggle}
        aria-label={item.text}
      />
      {editing ? (
        <input
          className="xc-input"
          value={text}
          autoFocus
          aria-label={t("Item text")}
          onChange={(e) => setText(e.target.value)}
          onBlur={() => onSave(text.trim(), false)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              onSave(text.trim(), true);
            }
            if (e.key === "Escape") {
              e.stopPropagation();
              setText(item.text);
              onEdit(false);
            }
          }}
        />
      ) : (
        <span
          className="projects-checklist-text"
          onClick={() => {
            setText(item.text);
            onEdit(true);
          }}
        >
          {item.text}
        </span>
      )}
      <span className="projects-checklist-actions">
        <button
          className="xc-btn ghost small"
          title={t("Convert to issue")}
          aria-label={`${t("Convert to issue")}: ${item.text}`}
          onClick={onConvert}
        >
          <ArrowUpRight size={13} />
        </button>
        <button
          className="xc-btn ghost small"
          title={t("Delete")}
          aria-label={`${t("Delete")}: ${item.text}`}
          onClick={onDelete}
        >
          <Trash2 size={13} />
        </button>
      </span>
    </li>
  );
}
