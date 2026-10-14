import { useEffect, useState } from "react";
import { Star, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateMeal,
  useDeleteMeal,
  useRecipes,
  useUpdateMeal,
  type Meal,
  type MealSlot,
} from "./api";
import { localDate, SLOT_LABELS, SLOTS, slotFor } from "./pick";

export interface MealDraft {
  date: string;
  slot: MealSlot;
  recipeId: number | null;
  title: string;
  cooked: boolean;
  rating: number | null;
  note: string;
}

export function newMealDraft(
  init: Partial<MealDraft> = {},
  now = new Date(),
): MealDraft {
  return {
    date: localDate(now),
    slot: slotFor(now),
    recipeId: null,
    title: "",
    cooked: false,
    rating: null,
    note: "",
    ...init,
  };
}

function fromMeal(m: Meal): MealDraft {
  return {
    date: m.date,
    slot: m.slot,
    recipeId: m.recipeId,
    title: m.title,
    cooked: m.cooked,
    rating: m.rating,
    note: m.note,
  };
}

/**
 * 记一顿（B210）。新建时传 initial，编辑时传 meal。
 * 做菜页的“做完了”也用它，那时 skipLabel 是“不记”，点了照样关掉并调用 onDone。
 */
export default function MealDialog({
  open,
  onClose,
  onDone,
  initial,
  meal,
  title,
  skipLabel,
}: {
  open: boolean;
  onClose: () => void;
  /** 记下来或者点了“不记”之后 */
  onDone?: () => void;
  initial?: Partial<MealDraft>;
  meal?: Meal | null;
  title?: string;
  skipLabel?: string;
}) {
  const t = useT();
  const recipes = useRecipes();
  const create = useCreateMeal();
  const update = useUpdateMeal();
  const remove = useDeleteMeal();
  const [d, setD] = useState<MealDraft>(() =>
    meal ? fromMeal(meal) : newMealDraft(initial),
  );
  // 每次打开都从头填
  useEffect(() => {
    if (open) setD(meal ? fromMeal(meal) : newMealDraft(initial));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, meal?.id]);

  const set = (patch: Partial<MealDraft>) => setD((v) => ({ ...v, ...patch }));
  const items = recipes.data?.items ?? [];
  const busy = create.isPending || update.isPending;
  const today = localDate(new Date());

  const finish = () => {
    onClose();
    onDone?.();
  };
  const fail = (err: unknown) =>
    toast({ message: errorMessage(err), tone: "error" });

  const save = () => {
    if (!d.recipeId && !d.title.trim()) {
      toast({
        message: t("Pick a recipe or write what you ate"),
        tone: "error",
      });
      return;
    }
    const body = {
      date: d.date,
      slot: d.slot,
      recipeId: d.recipeId,
      title: d.title.trim(),
      cooked: d.cooked,
      rating: d.rating,
      note: d.note.trim(),
    };
    if (meal)
      update.mutate(
        { id: meal.id, body },
        {
          onSuccess: () => {
            toast(t("Saved"));
            finish();
          },
          onError: fail,
        },
      );
    else
      create.mutate(body, {
        onSuccess: () => {
          toast(t("Meal recorded"));
          finish();
        },
        onError: fail,
      });
  };

  const onDelete = async () => {
    if (!meal) return;
    const ok = await confirmAction({
      title: `${t("Delete record")}：${meal.title}`,
      description: t(
        "The record is deleted. A habit check-in made for it stays.",
      ),
    });
    if (!ok) return;
    remove.mutate(meal.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        onClose();
      },
      onError: fail,
    });
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title ?? (meal ? t("Edit record") : t("Record a meal"))}
      footer={
        <>
          {meal ? (
            <button
              className="xc-btn danger recipes-meal-delete"
              onClick={() => void onDelete()}
            >
              <Trash2 size={14} /> {t("Delete")}
            </button>
          ) : null}
          <button className="xc-btn" onClick={skipLabel ? finish : onClose}>
            {skipLabel ?? t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={busy} onClick={save}>
            {t("Record it")}
          </button>
        </>
      }
    >
      <div className="recipes-meal-form">
        <div className="recipes-form-row">
          <label className="xc-field">
            <span>{t("Date")}</span>
            <input
              className="xc-input"
              type="date"
              max={today}
              value={d.date}
              onChange={(e) => set({ date: e.target.value })}
            />
          </label>
          <label className="xc-field">
            <span>{t("Meal")}</span>
            <select
              className="xc-select"
              value={d.slot}
              onChange={(e) => set({ slot: e.target.value as MealSlot })}
            >
              {SLOTS.map((s) => (
                <option key={s} value={s}>
                  {t(SLOT_LABELS[s])}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Recipe")}</span>
          <select
            className="xc-select"
            value={d.recipeId ?? ""}
            onChange={(e) => {
              const id = e.target.value ? Number(e.target.value) : null;
              const r = items.find((x) => x.id === id);
              set({
                recipeId: id,
                // 换了菜谱时，名称跟着换；手写过的名称不动
                title:
                  r && (!d.title || items.some((x) => x.title === d.title))
                    ? r.title
                    : d.title,
              });
            }}
          >
            <option value="">{t("No recipe")}</option>
            {items.map((r) => (
              <option key={r.id} value={r.id}>
                {r.title}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-field">
          <span>{t("What you ate")}</span>
          <input
            className="xc-input"
            value={d.title}
            maxLength={100}
            placeholder={t("For example: beef noodles from downstairs")}
            onChange={(e) => set({ title: e.target.value })}
          />
        </label>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={d.cooked}
            onChange={(e) => set({ cooked: e.target.checked })}
          />
          <span>{t("Cooked it myself")}</span>
        </label>
        <div className="xc-field">
          <span>{t("Rating")}</span>
          <StarRating value={d.rating} onChange={(rating) => set({ rating })} />
        </div>
        <label className="xc-field">
          <span>{t("Meal note")}</span>
          <input
            className="xc-input"
            value={d.note}
            maxLength={1000}
            placeholder={t("For example: a bit salty, use less salt next time")}
            onChange={(e) => set({ note: e.target.value })}
          />
        </label>
      </div>
    </Dialog>
  );
}

/** 1 到 5 颗星，再点一下同一颗清掉。 */
export function StarRating({
  value,
  onChange,
}: {
  value: number | null;
  onChange: (v: number | null) => void;
}) {
  const t = useT();
  return (
    <div className="recipes-stars" role="radiogroup" aria-label={t("Rating")}>
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          key={n}
          type="button"
          role="radio"
          aria-checked={value === n}
          aria-label={`${n} ${t("stars")}`}
          className={value != null && n <= value ? "on" : ""}
          onClick={() => onChange(value === n ? null : n)}
        >
          <Star size={18} />
        </button>
      ))}
    </div>
  );
}
