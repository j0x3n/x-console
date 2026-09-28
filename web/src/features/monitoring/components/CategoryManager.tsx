import { useState } from "react";
import { createPortal } from "react-dom";
import { Check, Pencil, Plus, Trash2, X } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useDeleteSubscriptionCategory,
  useSaveSubscriptionCategory,
  type SubscriptionCategoryItem,
} from "../api";
import { categoryLabels } from "./SubscriptionDialog";

/** 订阅分类管理（B23）：新建、改名、删除。“其他”不能删。 */
export default function CategoryManager({
  categories,
  onClose,
}: {
  categories: SubscriptionCategoryItem[];
  onClose: () => void;
}) {
  const t = useT();
  const save = useSaveSubscriptionCategory();
  const remove = useDeleteSubscriptionCategory();
  const [adding, setAdding] = useState("");
  const [editing, setEditing] = useState<{ id: number; name: string } | null>(
    null,
  );
  const fail = (err: unknown) =>
    toast({ message: errorMessage(err), tone: "error" });
  const label = (c: SubscriptionCategoryItem) =>
    c.builtin ? t(categoryLabels[c.builtin]) : c.name;

  const add = () => {
    const name = adding.trim();
    if (!name) return;
    save.mutate({ name }, { onSuccess: () => setAdding(""), onError: fail });
  };
  const rename = () => {
    if (!editing?.name.trim()) return;
    save.mutate(
      { id: editing.id, name: editing.name.trim() },
      { onSuccess: () => setEditing(null), onError: fail },
    );
  };
  const del = async (c: SubscriptionCategoryItem) => {
    if (
      !(await confirmAction({
        title: `${t("Delete")}“${label(c)}”？`,
        description:
          c.count > 0
            ? `${c.count} ${t("subscriptions in it move to Other.")}`
            : undefined,
      }))
    )
      return;
    remove.mutate(c.id, { onError: fail });
  };

  // 放到 body 下，不受外层弹窗的布局影响。
  return createPortal(
    <Dialog
      open
      onClose={onClose}
      title={t("Subscription categories")}
      footer={
        <button className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      }
    >
      <ul className="monitoring-categories">
        {categories.map((c) => (
          <li key={c.id}>
            {editing?.id === c.id ? (
              <>
                <input
                  className="xc-input"
                  value={editing.name}
                  autoFocus
                  aria-label={t("Category name")}
                  onChange={(e) =>
                    setEditing({ id: c.id, name: e.target.value })
                  }
                  onKeyDown={(e) => {
                    if (e.key === "Enter") rename();
                    if (e.key === "Escape") {
                      e.stopPropagation();
                      setEditing(null);
                    }
                  }}
                />
                <button
                  className="xc-btn ghost small"
                  aria-label={t("Save")}
                  onClick={rename}
                >
                  <Check size={14} />
                </button>
                <button
                  className="xc-btn ghost small"
                  aria-label={t("Cancel")}
                  onClick={() => setEditing(null)}
                >
                  <X size={14} />
                </button>
              </>
            ) : (
              <>
                <span className="monitoring-category-name">{label(c)}</span>
                <small className="xc-muted">{c.count}</small>
                <button
                  className="xc-btn ghost small"
                  aria-label={`${t("Rename")} ${label(c)}`}
                  onClick={() => setEditing({ id: c.id, name: label(c) })}
                >
                  <Pencil size={13} />
                </button>
                <button
                  className="xc-btn ghost small"
                  aria-label={`${t("Delete")} ${label(c)}`}
                  disabled={c.builtin === "other"}
                  onClick={() => del(c)}
                >
                  <Trash2 size={13} />
                </button>
              </>
            )}
          </li>
        ))}
      </ul>
      <div className="monitoring-category-add">
        <input
          className="xc-input"
          value={adding}
          maxLength={30}
          placeholder={t("New category, like Cloud storage")}
          aria-label={t("New category")}
          onChange={(e) => setAdding(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <button
          className="xc-btn small"
          disabled={!adding.trim() || save.isPending}
          onClick={add}
        >
          <Plus size={14} /> {t("Add")}
        </button>
      </div>
    </Dialog>,
    document.body,
  );
}
