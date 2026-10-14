import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import {
  ChefHat,
  Clock,
  FileInput,
  Plus,
  Settings,
  Sparkles,
  X,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCookTimers,
  useFillNutrition,
  useRecipes,
  useRecipeSettings,
  useRecommend,
  type Recipe,
  type RecipeNutrition,
  type RecipeRecommendation,
} from "./api";
import { useNow } from "./alarm";
import ImportDialog from "./ImportDialog";
import { addTag, filterRecipes, splitTags } from "./logic";
import {
  cookedText,
  doableRecipes,
  matchNutrition,
  NUTRITION,
  NUTRITION_LABELS,
  nutritionTone,
} from "./pick";
import SettingsDialog from "./SettingsDialog";
import TimerList from "./TimerList";
import "./i18n";
import "./recipes.css";

/**
 * 菜谱（B209）：全部菜谱的菜单，能按标签和关键字找。
 * B210 加了推荐、“我手头有”、营养分类筛选和设置。
 */
export default function RecipesPage() {
  const t = useT();
  const navigate = useNavigate();
  const list = useRecipes();
  const timers = useCookTimers().data ?? [];
  const now = useNow(timers.some((x) => x.state === "running"));
  const [tag, setTag] = useState("");
  const [q, setQ] = useState("");
  const [params, setParams] = useSearchParams();
  const importing = params.get("import") === "1";
  const setImporting = (on: boolean) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (on) next.set("import", "1");
        else next.delete("import");
        return next;
      },
      { replace: true },
    );
  const [showTimers, setShowTimers] = useState(false);
  const [have, setHave] = useState<string[]>([]);
  const [nutrition, setNutrition] = useState<RecipeNutrition[]>([]);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const recommend = useRecommend();
  const settings = useRecipeSettings();
  const fill = useFillNutrition();

  const all = list.data?.items ?? [];
  const { shown, more } = splitTags(list.data?.tags ?? [], tag);
  const filtered = filterRecipes(all, tag, q).filter((r) =>
    matchNutrition(r, nutrition),
  );
  // 填了“我手头有”时按还缺几样排，卡片上写缺什么
  const doable =
    have.length > 0
      ? doableRecipes(filtered, have, settings.data?.staples ?? [])
      : null;
  const items = doable ? doable.map((d) => d.recipe) : filtered;
  const missingOf = new Map(doable?.map((d) => [d.recipe.id, d.missing]));
  const filtering = have.length > 0 || nutrition.length > 0 || !!tag || !!q;
  const missingNutrition = all.filter((r) => !r.nutrition?.length).length;
  const suggestions = recommend.data?.items ?? [];

  const onFill = async () => {
    const n = Math.min(missingNutrition, 20);
    const ok = await confirmAction({
      title: t("Fill in nutrition with AI"),
      description: `${t("AI estimates the nutrition of")} ${n} ${t("recipes and saves it. Each one uses some AI credit.")}`,
      confirmLabel: t("Fill in now"),
      danger: false,
    });
    if (!ok) return;
    toast(t("AI is filling in nutrition. This can take a while."));
    fill.mutate(undefined, {
      onSuccess: (out) =>
        toast(
          `${t("Filled in")} ${out.filled}${out.failed ? `，${t("failed to fill")} ${out.failed}` : ""}${out.remaining ? `，${t("left")} ${out.remaining}` : ""}`,
        ),
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };

  const actions = (
    <>
      <button
        className="xc-btn"
        title={t("Import from text")}
        onClick={() => setImporting(true)}
      >
        <FileInput size={15} /> <span>{t("Import from text")}</span>
      </button>
      <button
        className="xc-btn primary"
        title={t("New recipe")}
        onClick={() => navigate("/recipes/new")}
      >
        <Plus size={15} /> <span>{t("New recipe")}</span>
      </button>
      <MoreMenu
        label={t("More")}
        title={t("Recipes")}
        items={[
          ...(missingNutrition > 0
            ? [
                {
                  key: "fill",
                  label: `${t("Fill in nutrition with AI")}（${missingNutrition}）`,
                  icon: <Sparkles size={14} />,
                  onSelect: () => void onFill(),
                },
              ]
            : []),
          {
            key: "settings",
            label: t("Recipe settings"),
            icon: <Settings size={14} />,
            onSelect: () => setSettingsOpen(true),
          },
        ]}
      />
    </>
  );

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Recipes")} icon={<ChefHat size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (all.length === 0)
    body = (
      <EmptyState title={t("No recipes yet")} icon={<ChefHat size={28} />}>
        <span>
          {t(
            "Write a recipe yourself, or paste a cooking guide and let AI turn it into a recipe. While cooking you get step timers and a list of what to add.",
          )}
        </span>
        <div className="xc-row">
          <button className="xc-btn small" onClick={() => setImporting(true)}>
            <FileInput size={14} /> {t("Import from text")}
          </button>
          <button
            className="xc-btn primary small"
            onClick={() => navigate("/recipes/new")}
          >
            <Plus size={14} /> {t("New recipe")}
          </button>
        </div>
      </EmptyState>
    );
  else if (items.length === 0)
    body = (
      <EmptyState title={t("No recipe matches")} icon={<ChefHat size={28} />} />
    );
  else
    body = (
      <div className="recipes-grid">
        {items.map((r) => (
          <RecipeCard key={r.id} recipe={r} missing={missingOf.get(r.id)} />
        ))}
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Recipes")}
        subtitle={
          timers.length > 0
            ? `${timers.length} ${t("timers running")}`
            : list.data
              ? `${all.length} ${t("recipes")}`
              : undefined
        }
        aside={actions}
      />
      <StatStrip label={t("Recipes")}>
        <StatCard
          label={t("All recipes")}
          value={list.data ? all.length : "–"}
        />
        <StatCard
          label={t("Timers running")}
          value={timers.length}
          tone={timers.length > 0 ? "accent" : undefined}
          onClick={
            timers.length > 0 ? () => setShowTimers((v) => !v) : undefined
          }
        />
      </StatStrip>
      {showTimers && timers.length > 0 && (
        <div className="xc-card recipes-timer-card">
          <TimerList timers={timers} now={now} showLinks />
        </div>
      )}
      {!filtering && suggestions.length > 0 && (
        <Suggestions
          items={suggestions}
          missingNutrition={recommend.data?.context.missingNutrition ?? 0}
          weightChange={recommend.data?.context.weightChange30d ?? null}
        />
      )}
      {all.length > 0 && (
        <Toolbar
          start={
            <div className="recipes-tags">
              <div className="xc-tabs" role="tablist" aria-label={t("Tags")}>
                <button
                  role="tab"
                  aria-selected={!tag}
                  className={!tag ? "active" : ""}
                  onClick={() => setTag("")}
                >
                  {t("All")}
                </button>
                {shown.map((x) => (
                  <button
                    key={x}
                    role="tab"
                    aria-selected={tag === x}
                    className={tag === x ? "active" : ""}
                    onClick={() => setTag(tag === x ? "" : x)}
                  >
                    {x}
                  </button>
                ))}
              </div>
              {more.length > 0 && (
                <select
                  className="xc-select"
                  aria-label={t("More tags")}
                  value=""
                  onChange={(e) => setTag(e.target.value)}
                >
                  <option value="">{t("More tags")}</option>
                  {more.map((x) => (
                    <option key={x} value={x}>
                      {x}
                    </option>
                  ))}
                </select>
              )}
            </div>
          }
          end={
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search recipes")}
              clearLabel={t("Clear")}
            />
          }
        />
      )}
      {all.length > 0 && (
        <div className="recipes-filters">
          <HaveInput have={have} onChange={setHave} />
          <div
            className="recipes-filter-group"
            role="group"
            aria-label={t("Nutrition")}
          >
            <span className="recipes-filter-label">{t("Nutrition")}</span>
            {NUTRITION.map((n) => (
              <button
                key={n}
                type="button"
                className={`recipes-pick${nutrition.includes(n) ? " on" : ""}`}
                aria-pressed={nutrition.includes(n)}
                onClick={() =>
                  setNutrition(
                    nutrition.includes(n)
                      ? nutrition.filter((x) => x !== n)
                      : [...nutrition, n],
                  )
                }
              >
                {t(NUTRITION_LABELS[n])}
              </button>
            ))}
          </div>
        </div>
      )}
      {body}
      <ImportDialog open={importing} onClose={() => setImporting(false)} />
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

/** “我手头有”：输入几样菜，回车加一个。 */
function HaveInput({
  have,
  onChange,
}: {
  have: string[];
  onChange: (v: string[]) => void;
}) {
  const t = useT();
  const [text, setText] = useState("");
  return (
    <div className="recipes-filter-group">
      <span className="recipes-filter-label">{t("I have")}</span>
      {have.map((x) => (
        <span key={x} className="xc-badge accent">
          {x}
          <button
            type="button"
            aria-label={`${t("Remove")} ${x}`}
            onClick={() => onChange(have.filter((y) => y !== x))}
          >
            <X size={11} />
          </button>
        </span>
      ))}
      <input
        className="xc-input recipes-have-input"
        value={text}
        maxLength={20}
        placeholder={t("For example: chicken. Press Enter")}
        aria-label={t("I have")}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault();
            onChange(addTag(have, text));
            setText("");
          }
          if (e.key === "Backspace" && !text && have.length)
            onChange(have.slice(0, -1));
        }}
        onBlur={() => {
          if (text.trim()) {
            onChange(addTag(have, text));
            setText("");
          }
        }}
      />
      {have.length > 0 && (
        <button
          type="button"
          className="xc-btn small ghost"
          onClick={() => onChange([])}
        >
          {t("Clear")}
        </button>
      )}
    </div>
  );
}

/** “今天可以做”：推荐的几道菜和理由。 */
function Suggestions({
  items,
  missingNutrition,
  weightChange,
}: {
  items: RecipeRecommendation[];
  missingNutrition: number;
  weightChange: number | null;
}) {
  const t = useT();
  return (
    <section className="recipes-suggest" aria-label={t("You could cook")}>
      <div className="recipes-suggest-head">
        <strong>{t("You could cook")}</strong>
        <small className="recipes-muted">
          {weightChange != null &&
            `${t("Weight in the last 30 days")} ${weightChange > 0 ? "+" : ""}${weightChange} kg`}
          {weightChange != null && missingNutrition > 0 && " · "}
          {missingNutrition > 0 &&
            t("Fill in nutrition to get better suggestions")}
        </small>
      </div>
      <div className="recipes-suggest-list">
        {items.map((it) => (
          <Link
            key={it.recipe.id}
            to={`/recipes/${it.recipe.id}`}
            className="recipes-suggest-item"
          >
            <strong>{it.recipe.title}</strong>
            {it.reasons.length > 0 && <small>{it.reasons.join(" · ")}</small>}
          </Link>
        ))}
      </div>
    </section>
  );
}

function RecipeCard({
  recipe: r,
  missing,
}: {
  recipe: Recipe;
  missing?: string[];
}) {
  const t = useT();
  const cooked = cookedText(r, t);
  return (
    <Link className="xc-card recipes-card" to={`/recipes/${r.id}`}>
      <strong className="recipes-card-title">{r.title}</strong>
      {r.summary && <p className="recipes-card-summary">{r.summary}</p>}
      {(r.tags.length > 0 || (r.nutrition ?? []).length > 0) && (
        <div className="recipes-card-tags">
          {r.tags.slice(0, 5).map((x) => (
            <span key={x} className="xc-badge">
              {x}
            </span>
          ))}
          {(r.nutrition ?? []).map((n) => (
            <span key={n} className={`xc-badge ${nutritionTone(n)}`}>
              {t(NUTRITION_LABELS[n])}
            </span>
          ))}
        </div>
      )}
      {missing && (
        <small className={`recipes-card-missing${missing.length ? "" : " ok"}`}>
          {missing.length
            ? `${t("Still need")}：${missing.join("、")}`
            : t("You have everything")}
        </small>
      )}
      <small className="recipes-card-meta">
        {r.totalMinutes != null && (
          <span>
            <Clock size={12} /> {r.totalMinutes} {t("min")}
          </span>
        )}
        <span>
          {r.ingredients.length} {t("ingredients")}
        </span>
        <span>
          {r.steps.length} {t("steps")}
        </span>
        {cooked && <span>{cooked}</span>}
      </small>
    </Link>
  );
}
