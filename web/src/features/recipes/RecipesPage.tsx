import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { ChefHat, Clock, FileInput, Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
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
import { useCookTimers, useRecipes, type Recipe } from "./api";
import { useNow } from "./alarm";
import ImportDialog from "./ImportDialog";
import { filterRecipes, splitTags } from "./logic";
import TimerList from "./TimerList";
import "./i18n";
import "./recipes.css";

/** 菜谱（B209）：全部菜谱的菜单，能按标签和关键字找。 */
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

  const all = list.data?.items ?? [];
  const { shown, more } = splitTags(list.data?.tags ?? [], tag);
  const items = filterRecipes(all, tag, q);

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
          <RecipeCard key={r.id} recipe={r} />
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
      {body}
      <ImportDialog open={importing} onClose={() => setImporting(false)} />
    </div>
  );
}

function RecipeCard({ recipe: r }: { recipe: Recipe }) {
  const t = useT();
  return (
    <Link className="xc-card recipes-card" to={`/recipes/${r.id}`}>
      <strong className="recipes-card-title">{r.title}</strong>
      {r.summary && <p className="recipes-card-summary">{r.summary}</p>}
      {r.tags.length > 0 && (
        <div className="recipes-card-tags">
          {r.tags.slice(0, 5).map((x) => (
            <span key={x} className="xc-badge">
              {x}
            </span>
          ))}
        </div>
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
      </small>
    </Link>
  );
}
