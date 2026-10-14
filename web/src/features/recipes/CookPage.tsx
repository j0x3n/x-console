import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router";
import {
  AlarmClock,
  ChevronLeft,
  ChevronRight,
  ChefHat,
  Check,
  ListChecks,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCookTimers, useRecipe, useStartTimer, type Recipe } from "./api";
import { unlockAudio, useNow } from "./alarm";
import { durationLabel } from "./durations";
import { loadStep, saveStep, stepIngredients, stepTimerChoices } from "./logic";
import MealDialog from "./MealDialog";
import TimerList from "./TimerList";
import "./i18n";
import "./recipes.css";

/** 做菜（B209）：一屏一步，计时按钮，这一步要加的料。 */
export default function CookPage() {
  const t = useT();
  const id = Number(useParams().id);
  const recipe = useRecipe(id);
  if (recipe.isPending) return <Loading />;
  if (recipe.isError)
    return (
      <div className="xc-page">
        {isNotLive(recipe.error) ? (
          <NotLive name={t("Recipes")} icon={<ChefHat size={28} />} />
        ) : (
          <ErrorState error={recipe.error} onRetry={() => recipe.refetch()} />
        )}
      </div>
    );
  return <Cook recipe={recipe.data} />;
}

/** 做菜时屏幕常亮。浏览器不支持就不管。 */
function useWakeLock() {
  useEffect(() => {
    const nav = navigator as Navigator & {
      wakeLock?: {
        request: (type: "screen") => Promise<{ release: () => Promise<void> }>;
      };
    };
    if (!nav.wakeLock) return;
    let lock: { release: () => Promise<void> } | null = null;
    let alive = true;
    const request = () => {
      if (document.visibilityState !== "visible") return;
      nav.wakeLock
        ?.request("screen")
        .then((l) => {
          if (alive) lock = l;
          else void l.release().catch(() => {});
        })
        .catch(() => {});
    };
    request();
    // 切走再回来时锁会被浏览器释放，要重新申请
    document.addEventListener("visibilitychange", request);
    return () => {
      alive = false;
      document.removeEventListener("visibilitychange", request);
      void lock?.release().catch(() => {});
    };
  }, []);
}

function Cook({ recipe: r }: { recipe: Recipe }) {
  const t = useT();
  const navigate = useNavigate();
  const count = r.steps.length;
  const [step, setStep] = useState(() => loadStep(r.id, count));
  const [showAll, setShowAll] = useState(false);
  const [logging, setLogging] = useState(false);
  const timers = useCookTimers().data ?? [];
  const now = useNow(timers.some((x) => x.state === "running"));
  const startTimer = useStartTimer();
  const touch = useRef<{ x: number; y: number } | null>(null);
  useWakeLock();

  const go = (next: number) => {
    const v = Math.max(0, Math.min(count - 1, next));
    setStep(v);
    saveStep(r.id, v);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return;
      if (e.key === "ArrowRight") go(step + 1);
      if (e.key === "ArrowLeft") go(step - 1);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  const finish = () => {
    saveStep(r.id, 0);
    navigate(`/recipes/${r.id}`);
  };

  const heading = (
    <PageHeading
      title={t("Cooking")}
      parents={[{ label: r.title, to: `/recipes/${r.id}` }]}
      aside={
        <button
          className={`xc-btn${showAll ? " primary" : ""}`}
          title={t("All ingredients")}
          aria-pressed={showAll}
          onClick={() => setShowAll((v) => !v)}
        >
          <ListChecks size={15} /> <span>{t("All ingredients")}</span>
        </button>
      }
    />
  );

  if (count === 0)
    return (
      <div className="xc-page">
        {heading}
        <div className="xc-empty">
          <ChefHat size={28} />
          <strong>{t("No steps yet")}</strong>
          <button
            className="xc-btn small"
            onClick={() => navigate(`/recipes/${r.id}/edit`)}
          >
            {t("Edit")}
          </button>
        </div>
      </div>
    );

  const s = r.steps[step];
  const ings = stepIngredients(s, r.ingredients);
  const choices = stepTimerChoices(s);
  const start = (label: string, seconds: number) => {
    unlockAudio();
    startTimer.mutate(
      { label, seconds, recipeId: r.id, stepIndex: step },
      {
        onSuccess: () =>
          toast(`${t("Timer started")}：${durationLabel(seconds)}`),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  return (
    <div className="xc-page recipes-cook">
      {heading}
      <div className="recipes-cook-progress">
        <span>
          第 {step + 1} / {count} 步
        </span>
        <div
          className="recipes-cook-bar"
          role="progressbar"
          aria-valuemin={1}
          aria-valuemax={count}
          aria-valuenow={step + 1}
        >
          <i style={{ width: `${((step + 1) / count) * 100}%` }} />
        </div>
      </div>

      {showAll && (
        <div className="xc-card recipes-cook-all">
          <ul className="recipes-ingredients">
            {r.ingredients.map((ing, i) => (
              <li key={i} className={ings.includes(i) ? "now" : ""}>
                <span>
                  {ing.name}
                  {ing.note && <small>{ing.note}</small>}
                </span>
                <span className="recipes-amount">{ing.amount}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <section
        className="xc-card recipes-cook-step"
        onTouchStart={(e) => {
          touch.current = {
            x: e.touches[0].clientX,
            y: e.touches[0].clientY,
          };
        }}
        onTouchEnd={(e) => {
          const from = touch.current;
          touch.current = null;
          if (!from) return;
          const dx = e.changedTouches[0].clientX - from.x;
          const dy = e.changedTouches[0].clientY - from.y;
          if (Math.abs(dx) > 60 && Math.abs(dx) > Math.abs(dy) * 2)
            go(dx < 0 ? step + 1 : step - 1);
        }}
      >
        {ings.length > 0 && (
          <div className="recipes-cook-add">
            <small>{t("Add in this step")}</small>
            <ul>
              {ings.map((k) => (
                <li key={k}>
                  <strong>{r.ingredients[k].name}</strong>
                  {r.ingredients[k].amount && (
                    <span>{r.ingredients[k].amount}</span>
                  )}
                  {r.ingredients[k].note && (
                    <small>{r.ingredients[k].note}</small>
                  )}
                </li>
              ))}
            </ul>
          </div>
        )}
        {s.title && <h2 className="recipes-cook-title">{s.title}</h2>}
        <p className="recipes-cook-text">{s.text}</p>
        {choices.length > 0 && (
          <div className="recipes-cook-timers">
            {choices.map((c) => (
              <button
                key={c.seconds}
                className="xc-btn recipes-cook-timer"
                disabled={startTimer.isPending}
                onClick={() => start(c.label, c.seconds)}
              >
                <AlarmClock size={16} /> {t("Timer for")}{" "}
                {durationLabel(c.seconds)}
              </button>
            ))}
          </div>
        )}
      </section>

      {timers.length > 0 && (
        <section className="xc-card recipes-cook-running">
          <div className="xc-card-head">
            <h2>{t("Cooking timers")}</h2>
          </div>
          <TimerList timers={timers} now={now} />
        </section>
      )}

      <nav className="recipes-cook-nav" aria-label={t("Recipe steps")}>
        <button
          className="xc-btn"
          disabled={step === 0}
          onClick={() => go(step - 1)}
        >
          <ChevronLeft size={18} /> {t("Previous step")}
        </button>
        {step < count - 1 ? (
          <button className="xc-btn primary" onClick={() => go(step + 1)}>
            {t("Next step")} <ChevronRight size={18} />
          </button>
        ) : (
          <button className="xc-btn primary" onClick={() => setLogging(true)}>
            <Check size={18} /> {t("All done")}
          </button>
        )}
      </nav>
      <MealDialog
        open={logging}
        onClose={() => setLogging(false)}
        onDone={finish}
        initial={{ recipeId: r.id, title: r.title, cooked: true }}
        title={t("Done. Record this meal?")}
        skipLabel={t("Do not record")}
      />
    </div>
  );
}
