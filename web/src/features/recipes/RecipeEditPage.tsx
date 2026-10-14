import { useEffect, useRef, useState } from "react";
import {
  useBlocker,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router";
import {
  ArrowDown,
  ArrowUp,
  ChefHat,
  Clock,
  Plus,
  Save,
  Trash2,
  WandSparkles,
  X,
} from "lucide-react";
import { errorMessage, isNotLive, unwrap } from "../../api/client";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useQuery } from "@tanstack/react-query";
import { notesApi, notesKeys } from "../notes/api";
import {
  useCreateRecipe,
  useEstimateNutrition,
  useParseRecipe,
  useRecipe,
  useUpdateCandidate,
  useUpdateRecipe,
  type RecipeInput,
  type RecipeNutrition,
  type RecipeStep,
  type RecipeTimer,
} from "./api";
import { ParseError } from "./ImportDialog";
import { NUTRITION, NUTRITION_LABELS } from "./pick";
import {
  addTag,
  emptyDraft,
  move,
  moveIngredient,
  removeIngredient,
  toDraft,
  toInput,
  type Draft,
} from "./logic";
import "./i18n";
import "./recipes.css";

/** 新建和编辑菜谱（B209）。/recipes/new、/recipes/:id/edit，都可以带 ?fromNote=笔记id。 */
export default function RecipeEditPage() {
  const t = useT();
  const id = Number(useParams().id ?? 0);
  const recipe = useRecipe(id);
  if (id > 0 && recipe.isPending) return <Loading />;
  if (id > 0 && recipe.isError)
    return (
      <div className="xc-page">
        {isNotLive(recipe.error) ? (
          <NotLive name={t("Recipes")} icon={<ChefHat size={28} />} />
        ) : (
          <ErrorState error={recipe.error} onRetry={() => recipe.refetch()} />
        )}
      </div>
    );
  return (
    <Editor
      key={id}
      id={id}
      initial={recipe.data ? toDraft(recipe.data) : null}
    />
  );
}

function Editor({ id, initial }: { id: number; initial: Draft | null }) {
  const t = useT();
  const navigate = useNavigate();
  const location = useLocation();
  const [params] = useSearchParams();
  const fromNote = Number(params.get("fromNote") ?? 0);
  const state = location.state as {
    draft?: RecipeInput;
    candidateId?: number;
  } | null;
  const stateDraft = state?.draft;
  // B211：从“找新菜谱”收进来的，保存后把候选标成已收下
  const candidateId = state?.candidateId ?? 0;

  const start = initial ?? (stateDraft ? toDraft(stateDraft) : emptyDraft());
  const [draft, setDraft] = useState<Draft>(start);
  const saved = useRef(JSON.stringify(initial ?? emptyDraft()));
  const [saving, setSaving] = useState(false);
  const create = useCreateRecipe();
  const update = useUpdateRecipe();
  const accept = useUpdateCandidate();
  const dirty = JSON.stringify(draft) !== saved.current;

  // 从笔记转换：读笔记正文，交给 AI 整理，结果填进表单
  const note = useQuery({
    queryKey: notesKeys.note(fromNote),
    queryFn: () =>
      unwrap(
        notesApi.GET("/notes/{noteId}", {
          params: { path: { noteId: fromNote } },
        }),
      ),
    enabled: fromNote > 0,
  });
  const parse = useParseRecipe();
  const parsed = useRef(false);
  useEffect(() => {
    if (!fromNote || parsed.current || !note.data) return;
    parsed.current = true;
    parse.mutate(
      { text: note.data.body, sourceNoteId: fromNote },
      {
        onSuccess: (out) =>
          setDraft((d) => ({
            ...toDraft(out),
            sourceNoteId: fromNote,
            sourceUrl: out.sourceUrl || d.sourceUrl,
          })),
      },
    );
  }, [fromNote, note.data, parse]);

  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      dirty && !saving && currentLocation.pathname !== nextLocation.pathname,
  );
  useEffect(() => {
    if (blocker.state !== "blocked") return;
    void confirmAction({
      title: t("Leave without saving?"),
      description: t("Changes to this recipe are not saved yet."),
      confirmLabel: t("Leave"),
    }).then((ok) => (ok ? blocker.proceed() : blocker.reset()));
  }, [blocker, t]);
  useEffect(() => {
    if (!dirty) return;
    const onUnload = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, [dirty]);

  const set = (patch: Partial<Draft>) => setDraft((d) => ({ ...d, ...patch }));

  const onSave = () => {
    const body = toInput(draft);
    if (!body.title) {
      toast({ message: t("Give the recipe a name"), tone: "error" });
      return;
    }
    setSaving(true);
    const done = (rid: number) => {
      saved.current = JSON.stringify(draft);
      toast(t("Saved"));
      navigate(`/recipes/${rid}`, { replace: true });
    };
    const fail = (err: unknown) => {
      setSaving(false);
      toast({ message: errorMessage(err), tone: "error" });
    };
    if (id > 0)
      update.mutate(
        { id, body: { ...body, totalMinutes: body.totalMinutes ?? null } },
        { onSuccess: (r) => done(r.id), onError: fail },
      );
    else
      create.mutate(body, {
        onSuccess: (r) => {
          if (candidateId > 0)
            accept.mutate({
              id: candidateId,
              status: "accepted",
              recipeId: r.id,
            });
          done(r.id);
        },
        onError: fail,
      });
  };

  const pageTitle = id > 0 ? draft.title || t("Edit recipe") : t("New recipe");
  return (
    <div className="xc-page recipes-edit">
      <PageHeading
        title={pageTitle}
        aside={
          <button
            className="xc-btn primary"
            title={t("Save")}
            disabled={saving}
            onClick={onSave}
          >
            <Save size={15} /> <span>{t("Save")}</span>
          </button>
        }
      />
      {fromNote > 0 && (parse.isPending || note.isPending) && (
        <div className="xc-card recipes-parsing">
          <WandSparkles size={15} />
          <span>{t("AI is turning the note into a recipe…")}</span>
        </div>
      )}
      {fromNote > 0 && note.isError && (
        <div className="xc-card recipes-parsing">
          <span>{errorMessage(note.error)}</span>
        </div>
      )}
      {parse.isError && (
        <div className="xc-card recipes-parsing">
          <ParseError error={parse.error} />
        </div>
      )}
      {parse.isSuccess && (
        <div className="xc-card recipes-parsing ok">
          <WandSparkles size={15} />
          <span>{t("AI filled in the form. Check it, then save.")}</span>
        </div>
      )}

      <section className="xc-card recipes-form">
        <label className="xc-field">
          <span>{t("Recipe name")}</span>
          <input
            className="xc-input"
            value={draft.title}
            maxLength={100}
            onChange={(e) => set({ title: e.target.value })}
          />
        </label>
        <label className="xc-field">
          <span>{t("Summary")}</span>
          <input
            className="xc-input"
            value={draft.summary}
            maxLength={500}
            onChange={(e) => set({ summary: e.target.value })}
          />
        </label>
        <div className="recipes-form-row">
          <label className="xc-field">
            <span>{t("Servings")}</span>
            <input
              className="xc-input"
              value={draft.servings}
              maxLength={50}
              placeholder={t("For example: 2 people")}
              onChange={(e) => set({ servings: e.target.value })}
            />
          </label>
          <label className="xc-field">
            <span>{t("Time needed (minutes)")}</span>
            <input
              className="xc-input"
              type="number"
              min={1}
              max={10080}
              value={draft.totalMinutes ?? ""}
              onChange={(e) =>
                set({
                  totalMinutes: e.target.value
                    ? Math.max(1, Math.round(Number(e.target.value)))
                    : null,
                })
              }
            />
          </label>
        </div>
        <TagInput tags={draft.tags} onChange={(tags) => set({ tags })} />
        <NutritionPicker
          id={id}
          value={draft.nutrition}
          onChange={(nutrition) => set({ nutrition })}
        />
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Ingredients")}</h2>
        </div>
        <ul className="recipes-ing-edit">
          {draft.ingredients.map((ing, i) => (
            <li key={i}>
              <input
                className="xc-input recipes-ing-amount"
                placeholder={t("Ingredient amount")}
                aria-label={t("Ingredient amount")}
                value={ing.amount ?? ""}
                maxLength={30}
                onChange={(e) =>
                  set({
                    ingredients: draft.ingredients.map((x, k) =>
                      k === i ? { ...x, amount: e.target.value } : x,
                    ),
                  })
                }
              />
              <input
                className="xc-input recipes-ing-name"
                placeholder={t("Ingredient")}
                aria-label={t("Ingredient")}
                value={ing.name}
                maxLength={50}
                onChange={(e) =>
                  set({
                    ingredients: draft.ingredients.map((x, k) =>
                      k === i ? { ...x, name: e.target.value } : x,
                    ),
                  })
                }
              />
              <input
                className="xc-input recipes-ing-note"
                placeholder={t("Preparation")}
                aria-label={t("Preparation")}
                value={ing.note ?? ""}
                maxLength={100}
                onChange={(e) =>
                  set({
                    ingredients: draft.ingredients.map((x, k) =>
                      k === i ? { ...x, note: e.target.value } : x,
                    ),
                  })
                }
              />
              <RowButtons
                first={i === 0}
                last={i === draft.ingredients.length - 1}
                onUp={() => setDraft((d) => moveIngredient(d, i, i - 1))}
                onDown={() => setDraft((d) => moveIngredient(d, i, i + 1))}
                onRemove={() => setDraft((d) => removeIngredient(d, i))}
              />
            </li>
          ))}
        </ul>
        <button
          className="xc-btn small"
          disabled={draft.ingredients.length >= 100}
          onClick={() =>
            set({
              ingredients: [
                ...draft.ingredients,
                { name: "", amount: "", note: "" },
              ],
            })
          }
        >
          <Plus size={14} /> {t("Add ingredient")}
        </button>
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Recipe steps")}</h2>
        </div>
        <ol className="recipes-step-edit">
          {draft.steps.map((s, i) => (
            <StepEditor
              key={i}
              index={i}
              step={s}
              draft={draft}
              onChange={(step) =>
                set({
                  steps: draft.steps.map((x, k) => (k === i ? step : x)),
                })
              }
              first={i === 0}
              last={i === draft.steps.length - 1}
              onUp={() => set({ steps: move(draft.steps, i, i - 1) })}
              onDown={() => set({ steps: move(draft.steps, i, i + 1) })}
              onRemove={() =>
                set({ steps: draft.steps.filter((_, k) => k !== i) })
              }
            />
          ))}
        </ol>
        <button
          className="xc-btn small"
          disabled={draft.steps.length >= 100}
          onClick={() =>
            set({
              steps: [
                ...draft.steps,
                { title: "", text: "", timers: [], ingredients: [] },
              ],
            })
          }
        >
          <Plus size={14} /> {t("Add step")}
        </button>
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Notes and tips")}</h2>
        </div>
        <div className="xc-field">
          <MarkdownEditor
            value={draft.tips}
            onChange={(tips) => set({ tips })}
            label={t("Notes and tips")}
            minRows={6}
          />
        </div>
        <label className="xc-field">
          <span>{t("Source link")}</span>
          <input
            className="xc-input"
            type="url"
            placeholder="https://"
            value={draft.sourceUrl}
            onChange={(e) => set({ sourceUrl: e.target.value })}
          />
        </label>
      </section>

      <div className="recipes-edit-foot">
        <button className="xc-btn" onClick={() => navigate(-1)}>
          {t("Cancel")}
        </button>
        <button className="xc-btn primary" disabled={saving} onClick={onSave}>
          <Save size={14} /> {t("Save")}
        </button>
      </div>
    </div>
  );
}

function RowButtons({
  first,
  last,
  onUp,
  onDown,
  onRemove,
}: {
  first: boolean;
  last: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
}) {
  const t = useT();
  return (
    <span className="recipes-row-buttons">
      <button
        className="xc-btn small ghost"
        title={t("Move up")}
        aria-label={t("Move up")}
        disabled={first}
        onClick={onUp}
      >
        <ArrowUp size={14} />
      </button>
      <button
        className="xc-btn small ghost"
        title={t("Move down")}
        aria-label={t("Move down")}
        disabled={last}
        onClick={onDown}
      >
        <ArrowDown size={14} />
      </button>
      <button
        className="xc-btn small ghost"
        title={t("Remove")}
        aria-label={t("Remove")}
        onClick={onRemove}
      >
        <Trash2 size={14} />
      </button>
    </span>
  );
}

function StepEditor({
  index,
  step,
  draft,
  onChange,
  first,
  last,
  onUp,
  onDown,
  onRemove,
}: {
  index: number;
  step: RecipeStep;
  draft: Draft;
  onChange: (s: RecipeStep) => void;
  first: boolean;
  last: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
}) {
  const t = useT();
  const timers = step.timers ?? [];
  const used = new Set(step.ingredients ?? []);
  const setTimer = (k: number, patch: Partial<RecipeTimer>) =>
    onChange({
      ...step,
      timers: timers.map((x, j) => (j === k ? { ...x, ...patch } : x)),
    });
  return (
    <li className="recipes-step-card">
      <div className="recipes-step-card-head">
        <span className="recipes-step-no">{index + 1}</span>
        <input
          className="xc-input"
          placeholder={t("Step title (optional)")}
          aria-label={t("Step title (optional)")}
          value={step.title ?? ""}
          maxLength={30}
          onChange={(e) => onChange({ ...step, title: e.target.value })}
        />
        <RowButtons
          first={first}
          last={last}
          onUp={onUp}
          onDown={onDown}
          onRemove={onRemove}
        />
      </div>
      <textarea
        className="xc-textarea"
        rows={3}
        aria-label={t("What to do")}
        placeholder={t("What to do")}
        value={step.text}
        maxLength={2000}
        onChange={(e) => onChange({ ...step, text: e.target.value })}
      />
      {timers.map((tm, k) => (
        <div key={k} className="recipes-timer-edit">
          <Clock size={14} />
          <input
            className="xc-input"
            aria-label={t("Timer name")}
            placeholder={t("Timer name")}
            value={tm.label}
            maxLength={30}
            onChange={(e) => setTimer(k, { label: e.target.value })}
          />
          <MinutesInput
            label={t("Minutes")}
            seconds={tm.seconds}
            onChange={(seconds) => setTimer(k, { seconds: seconds ?? 0 })}
          />
          <span className="recipes-muted">{t("to")}</span>
          <MinutesInput
            label={t("Up to (optional)")}
            seconds={tm.maxSeconds ?? null}
            onChange={(maxSeconds) => setTimer(k, { maxSeconds })}
          />
          <span className="recipes-muted">{t("min")}</span>
          <button
            className="xc-btn small ghost"
            title={t("Remove")}
            aria-label={t("Remove")}
            onClick={() =>
              onChange({ ...step, timers: timers.filter((_, j) => j !== k) })
            }
          >
            <X size={14} />
          </button>
        </div>
      ))}
      {draft.ingredients.some((x) => x.name.trim()) && (
        <div className="recipes-step-pick">
          <small>{t("Added in this step")}</small>
          <div>
            {draft.ingredients.map((ing, k) =>
              ing.name.trim() ? (
                <button
                  key={k}
                  type="button"
                  className={`recipes-pick${used.has(k) ? " on" : ""}`}
                  aria-pressed={used.has(k)}
                  onClick={() => {
                    const next = new Set(used);
                    if (next.has(k)) next.delete(k);
                    else next.add(k);
                    onChange({
                      ...step,
                      ingredients: [...next].sort((a, b) => a - b),
                    });
                  }}
                >
                  {ing.name}
                </button>
              ) : null,
            )}
          </div>
        </div>
      )}
      {timers.length < 5 && (
        <button
          className="xc-btn small ghost recipes-add-timer"
          onClick={() =>
            onChange({
              ...step,
              timers: [
                ...timers,
                {
                  label: (step.title ?? "").trim() || t("Timer"),
                  seconds: 300,
                },
              ],
            })
          }
        >
          <Plus size={14} /> {t("Add timer")}
        </button>
      )}
    </li>
  );
}

/** 按分钟填，存成秒。可以填小数，比如 0.5 是 30 秒。 */
function MinutesInput({
  label,
  seconds,
  onChange,
}: {
  label: string;
  seconds: number | null;
  onChange: (seconds: number | null) => void;
}) {
  const [text, setText] = useState(seconds ? String(seconds / 60) : "");
  useEffect(() => {
    const current = text ? Math.round(Number(text) * 60) : null;
    if (current !== seconds) setText(seconds ? String(seconds / 60) : "");
    // 只在外面的值变了时同步
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seconds]);
  return (
    <input
      className="xc-input recipes-minutes"
      type="number"
      min={0}
      step="any"
      inputMode="decimal"
      aria-label={label}
      placeholder={label}
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        onChange(
          e.target.value && Number.isFinite(n) && n > 0
            ? Math.round(n * 60)
            : null,
        );
      }}
    />
  );
}

function TagInput({
  tags,
  onChange,
}: {
  tags: string[];
  onChange: (tags: string[]) => void;
}) {
  const t = useT();
  const [text, setText] = useState("");
  const commit = () => {
    onChange(addTag(tags, text));
    setText("");
  };
  return (
    <div className="xc-field">
      <span>{t("Tags")}</span>
      <div className="recipes-tag-input">
        {tags.map((x) => (
          <span key={x} className="xc-badge">
            {x}
            <button
              type="button"
              aria-label={`${t("Remove")} ${x}`}
              onClick={() => onChange(tags.filter((y) => y !== x))}
            >
              <X size={11} />
            </button>
          </span>
        ))}
        {tags.length < 20 && (
          <input
            className="xc-input"
            value={text}
            maxLength={20}
            placeholder={t("Taste, cuisine, main ingredient. Press Enter")}
            aria-label={t("Add tag")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                e.preventDefault();
                commit();
              }
            }}
            onBlur={() => text.trim() && commit()}
          />
        )}
      </div>
    </div>
  );
}

/** 营养分类（B210）：点选，编辑已有菜谱时可以让 AI 估一下。 */
function NutritionPicker({
  id,
  value,
  onChange,
}: {
  id: number;
  value: RecipeNutrition[];
  onChange: (v: RecipeNutrition[]) => void;
}) {
  const t = useT();
  const estimate = useEstimateNutrition();
  return (
    <div className="xc-field">
      <span>{t("Nutrition")}</span>
      <div className="recipes-step-pick">
        <div>
          {NUTRITION.map((n) => (
            <button
              key={n}
              type="button"
              className={`recipes-pick${value.includes(n) ? " on" : ""}`}
              aria-pressed={value.includes(n)}
              onClick={() =>
                onChange(
                  value.includes(n)
                    ? value.filter((x) => x !== n)
                    : NUTRITION.filter((x) => x === n || value.includes(x)),
                )
              }
            >
              {t(NUTRITION_LABELS[n])}
            </button>
          ))}
          {id > 0 && (
            <button
              type="button"
              className="xc-btn small ghost"
              disabled={estimate.isPending}
              onClick={() =>
                estimate.mutate(id, {
                  onSuccess: (out) => {
                    onChange(out.nutrition);
                    toast(
                      t("AI filled in the nutrition. Check it, then save."),
                    );
                  },
                  onError: (err) =>
                    toast({ message: errorMessage(err), tone: "error" }),
                })
              }
            >
              <WandSparkles size={14} />
              {estimate.isPending ? t("AI is working…") : t("Estimate with AI")}
            </button>
          )}
        </div>
      </div>
      <small>
        {t(
          "A rough guess from the ingredients and amounts, not a nutrition table.",
        )}
      </small>
    </div>
  );
}
