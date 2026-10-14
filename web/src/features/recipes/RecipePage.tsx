import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  ChefHat,
  Clock,
  ExternalLink,
  NotebookPen,
  Pencil,
  RefreshCw,
  Trash2,
  UtensilsCrossed,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useDeleteRecipe, useRecipe, type Recipe } from "./api";
import { durationLabel } from "./durations";
import { stepIngredients } from "./logic";
import MealDialog from "./MealDialog";
import { NUTRITION_LABELS, nutritionTone } from "./pick";
import "./i18n";
import "./recipes.css";

/** 一份菜谱的详情（B209）：步骤在左，食材和属性在右。 */
export default function RecipePage() {
  const t = useT();
  const id = Number(useParams().id);
  const recipe = useRecipe(id);
  if (recipe.isPending) return <Loading />;
  if (recipe.isError && isNotLive(recipe.error))
    return (
      <div className="xc-page">
        <NotLive name={t("Recipes")} icon={<ChefHat size={28} />} />
      </div>
    );
  if (recipe.isError)
    return (
      <div className="xc-page">
        <ErrorState error={recipe.error} onRetry={() => recipe.refetch()} />
      </div>
    );
  return <RecipeView recipe={recipe.data} />;
}

function RecipeView({ recipe: r }: { recipe: Recipe }) {
  const t = useT();
  const navigate = useNavigate();
  const remove = useDeleteRecipe();
  const [logging, setLogging] = useState(false);

  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete recipe")}：${r.title}`,
      description: t(
        "The recipe is deleted. Timers that are running keep going.",
      ),
    });
    if (!ok) return;
    remove.mutate(r.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        navigate("/recipes");
      },
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };

  const more = [
    {
      key: "log",
      label: t("Record a meal"),
      icon: <UtensilsCrossed size={14} />,
      onSelect: () => setLogging(true),
    },
    ...(r.sourceNoteId
      ? [
          {
            key: "reparse",
            label: t("Convert from the note again"),
            icon: <RefreshCw size={14} />,
            onSelect: () =>
              navigate(`/recipes/${r.id}/edit?fromNote=${r.sourceNoteId}`),
          },
        ]
      : []),
    {
      key: "delete",
      label: t("Delete"),
      icon: <Trash2 size={14} />,
      danger: true,
      onSelect: () => void onDelete(),
    },
  ];

  return (
    <div className="xc-page">
      <PageHeading
        title={r.title}
        aside={
          <>
            <button
              className="xc-btn"
              title={t("Edit")}
              onClick={() => navigate(`/recipes/${r.id}/edit`)}
            >
              <Pencil size={15} /> <span>{t("Edit")}</span>
            </button>
            <button
              className="xc-btn primary"
              title={t("Start cooking")}
              onClick={() => navigate(`/recipes/${r.id}/cook`)}
            >
              <ChefHat size={15} /> <span>{t("Start cooking")}</span>
            </button>
            <MoreMenu items={more} title={r.title} label={t("More")} />
          </>
        }
      />
      <div className="recipes-detail">
        <main className="recipes-detail-main">
          {r.summary && <p className="recipes-detail-summary">{r.summary}</p>}
          <section className="recipes-section">
            <h2>{t("Recipe steps")}</h2>
            {r.steps.length === 0 ? (
              <p className="recipes-muted">{t("No steps yet")}</p>
            ) : (
              <ol className="recipes-steps">
                {r.steps.map((s, i) => {
                  const ings = stepIngredients(s, r.ingredients);
                  return (
                    <li key={i} className="recipes-step">
                      <span className="recipes-step-no">{i + 1}</span>
                      <div className="recipes-step-body">
                        {s.title && <strong>{s.title}</strong>}
                        <p>{s.text}</p>
                        {(ings.length > 0 || (s.timers ?? []).length > 0) && (
                          <div className="recipes-step-chips">
                            {ings.map((k) => (
                              <span key={`i${k}`} className="xc-badge">
                                {r.ingredients[k].name}
                                {r.ingredients[k].amount
                                  ? ` ${r.ingredients[k].amount}`
                                  : ""}
                              </span>
                            ))}
                            {(s.timers ?? []).map((tm, k) => (
                              <span key={`t${k}`} className="xc-badge accent">
                                <Clock size={11} /> {tm.label}{" "}
                                {durationLabel(tm.seconds)}
                                {tm.maxSeconds
                                  ? ` – ${durationLabel(tm.maxSeconds)}`
                                  : ""}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                    </li>
                  );
                })}
              </ol>
            )}
          </section>
          {r.tips.trim() && (
            <section className="recipes-section">
              <h2>{t("Notes and tips")}</h2>
              <Markdown source={r.tips} className="recipes-tips" />
            </section>
          )}
        </main>
        <aside className="recipes-detail-side">
          <div className="recipes-props">
            <dl className="recipes-meta">
              {r.servings && (
                <>
                  <dt>{t("Servings")}</dt>
                  <dd>{r.servings}</dd>
                </>
              )}
              {r.cookedCount > 0 && (
                <>
                  <dt>{t("Cooked")}</dt>
                  <dd>
                    {r.cookedCount} {t("times")}
                    {r.avgRating != null && ` · ${r.avgRating} ${t("stars")}`}
                  </dd>
                </>
              )}
              {r.lastCookedOn && (
                <>
                  <dt>{t("Last cooked")}</dt>
                  <dd>{r.lastCookedOn}</dd>
                </>
              )}
              {r.totalMinutes != null && (
                <>
                  <dt>{t("Time needed")}</dt>
                  <dd>
                    {r.totalMinutes} {t("min")}
                  </dd>
                </>
              )}
            </dl>
            {r.tags.length > 0 && (
              <div className="recipes-card-tags">
                {r.tags.map((x) => (
                  <span key={x} className="xc-badge">
                    {x}
                  </span>
                ))}
              </div>
            )}
            {(r.nutrition ?? []).length > 0 && (
              <div>
                <h3>
                  {t("Nutrition")}
                  <small className="recipes-muted">
                    {" "}
                    · {t("estimated by AI")}
                  </small>
                </h3>
                <div className="recipes-card-tags">
                  {r.nutrition.map((n) => (
                    <span key={n} className={`xc-badge ${nutritionTone(n)}`}>
                      {t(NUTRITION_LABELS[n])}
                    </span>
                  ))}
                </div>
              </div>
            )}
            <div>
              <h3>{t("Ingredients")}</h3>
              {r.ingredients.length === 0 ? (
                <p className="recipes-muted">{t("No ingredients yet")}</p>
              ) : (
                <ul className="recipes-ingredients">
                  {r.ingredients.map((ing, i) => (
                    <li key={i}>
                      <span>
                        {ing.name}
                        {ing.note && <small>{ing.note}</small>}
                      </span>
                      <span className="recipes-amount">{ing.amount}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            {(r.sourceNoteId || r.sourceUrl) && (
              <div className="recipes-sources">
                <h3>{t("Source")}</h3>
                {r.sourceNoteId && (
                  <Link to={`/notes/${r.sourceNoteId}`}>
                    <NotebookPen size={13} /> {t("Source note")}
                  </Link>
                )}
                {r.sourceUrl && (
                  <a href={r.sourceUrl} target="_blank" rel="noreferrer">
                    <ExternalLink size={13} /> {hostOf(r.sourceUrl)}
                  </a>
                )}
              </div>
            )}
          </div>
        </aside>
      </div>
      <MealDialog
        open={logging}
        onClose={() => setLogging(false)}
        initial={{ recipeId: r.id, title: r.title, cooked: true }}
      />
    </div>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
