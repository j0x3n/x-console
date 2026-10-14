import { useState } from "react";
import { Link } from "react-router";
import { ChefHat, Plus, Star, UtensilsCrossed } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useMeals, type Meal } from "./api";
import MealDialog from "./MealDialog";
import { daysAgo, groupMeals, localDate, SLOT_LABELS } from "./pick";
import "./i18n";
import "./recipes.css";

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

/** “10月14日 周二”，今天和昨天直接写出来。 */
function dayTitle(date: string, today: string, yesterday: string): string {
  if (date === today) return "今天";
  if (date === yesterday) return "昨天";
  const [y, m, d] = date.split("-").map(Number);
  const w = new Date(y, m - 1, d).getDay();
  return `${m}月${d}日 周${WEEKDAYS[w]}`;
}

/** 饮食记录（B210）：最近 30 天每天吃了什么，有体重的日子写在标题后面。 */
export default function MealsPage() {
  const t = useT();
  const now = new Date();
  const today = localDate(now);
  const yesterday = daysAgo(now, 1);
  const list = useMeals();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Meal | null>(null);

  const meals = list.data?.items ?? [];
  const days = groupMeals(meals, list.data?.weights ?? []);
  const cooked = meals.filter((m) => m.cooked).length;
  const rated = meals.filter((m) => m.rating != null);
  const avg =
    rated.length > 0
      ? Math.round(
          (rated.reduce((s, m) => s + (m.rating ?? 0), 0) / rated.length) * 10,
        ) / 10
      : null;
  const weights = list.data?.weights ?? [];
  const lastWeight = weights.length ? weights[weights.length - 1] : null;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = (
      <NotLive name={t("Meal log")} icon={<UtensilsCrossed size={28} />} />
    );
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (days.length === 0)
    body = (
      <EmptyState
        title={t("Nothing recorded in the last 30 days")}
        icon={<UtensilsCrossed size={28} />}
      >
        <span>
          {t(
            "Record what you eat. Meals you cook from a recipe are recorded when you tap All done on the cooking page.",
          )}
        </span>
        <button
          className="xc-btn primary small"
          onClick={() => setCreating(true)}
        >
          <Plus size={14} /> {t("Record a meal")}
        </button>
      </EmptyState>
    );
  else
    body = (
      <div className="recipes-days">
        {days.map((day) => (
          <section key={day.date} className="xc-card recipes-day">
            <div className="recipes-day-head">
              <strong>{dayTitle(day.date, today, yesterday)}</strong>
              {day.weight != null && (
                <span className="recipes-muted">
                  {t("Weight")} {day.weight} kg
                </span>
              )}
            </div>
            {day.slots.map((g) => (
              <div key={g.slot} className="recipes-day-slot">
                <span className="recipes-day-slot-name">
                  {t(SLOT_LABELS[g.slot])}
                </span>
                <ul>
                  {g.meals.map((m) => (
                    <li key={m.id}>
                      <button
                        className="recipes-meal-row"
                        onClick={() => setEditing(m)}
                      >
                        <span className="recipes-meal-title">{m.title}</span>
                        {m.cooked && (
                          <span className="xc-badge accent">
                            <ChefHat size={11} /> {t("Cooked it myself")}
                          </span>
                        )}
                        {m.rating != null && (
                          <span className="recipes-meal-rating">
                            <Star size={12} /> {m.rating}
                          </span>
                        )}
                        {m.note && (
                          <small className="recipes-meal-note">{m.note}</small>
                        )}
                      </button>
                      {m.recipeId && (
                        <Link
                          className="recipes-meal-link"
                          to={`/recipes/${m.recipeId}`}
                        >
                          {t("Recipe")}
                        </Link>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </section>
        ))}
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Meal log")}
        subtitle={t("Last 30 days")}
        aside={
          <button
            className="xc-btn primary"
            title={t("Record a meal")}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} /> <span>{t("Record a meal")}</span>
          </button>
        }
      />
      <StatStrip label={t("Meal log")}>
        <StatCard label={t("Records")} value={list.data ? meals.length : "–"} />
        <StatCard
          label={t("Cooked it myself")}
          value={list.data ? cooked : "–"}
        />
        <StatCard label={t("Average rating")} value={avg ?? "–"} />
        <StatCard
          label={t("Latest weight")}
          value={lastWeight ? lastWeight.weight : "–"}
          unit={lastWeight ? "kg" : undefined}
          foot={
            weights.length >= 2
              ? `${t("30-day change")} ${signed(
                  lastWeight!.weight - weights[0].weight,
                )} kg`
              : undefined
          }
        />
      </StatStrip>
      <p className="recipes-note">
        {t(
          "Weight is shown next to what you ate for the same day. Many things change weight, so this page does not say which dish caused it.",
        )}
      </p>
      {body}
      <MealDialog open={creating} onClose={() => setCreating(false)} />
      <MealDialog
        open={editing !== null}
        meal={editing}
        onClose={() => setEditing(null)}
      />
    </div>
  );
}

function signed(n: number): string {
  const v = Math.round(n * 10) / 10;
  return v > 0 ? `+${v}` : String(v);
}
