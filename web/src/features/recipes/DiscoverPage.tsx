import { useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Globe,
  Plus,
  Search,
  X,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCandidates,
  useDiscover,
  useUpdateCandidate,
  type DietGoal,
  type RecipeCandidate,
} from "./api";
import { NUTRITION_LABELS, nutritionTone } from "./pick";
import "./i18n";
import "./recipes.css";

const GOAL_LABELS: Record<DietGoal, string> = {
  none: "No goal",
  lose: "Lose weight",
  gain: "Build muscle",
  steady: "Keep my weight",
};

/** 找新菜谱（B211）：按口味联网找，结果放进待确认，收进菜单才保存。 */
export default function DiscoverPage() {
  const t = useT();
  const list = useCandidates();
  const discover = useDiscover();
  const [hint, setHint] = useState("");
  const [count, setCount] = useState(3);

  const profile = list.data?.profile;
  const items = list.data?.items ?? [];
  const off = profile?.webSearch === "off";

  const run = () =>
    discover.mutate(
      { hint: hint.trim() || undefined, count },
      {
        onSuccess: (out) =>
          toast(
            out.items.length
              ? `${t("Found")} ${out.items.length} ${t("new recipes")}`
              : t("Nothing suitable this time. Try asking another way."),
          ),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Find new recipes")} icon={<Globe size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0)
    // 联网搜索关着时上面已经有说明，这里不再重复
    body = off ? null : (
      <EmptyState title={t("No recipes waiting")} icon={<Globe size={28} />}>
        <span>
          {t(
            "Recipes found online wait here. Add the ones you like to your menu.",
          )}
        </span>
      </EmptyState>
    );
  else
    body = (
      <div className="recipes-candidates">
        {items.map((c) => (
          <CandidateCard key={c.id} candidate={c} />
        ))}
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Find new recipes")}
        subtitle={
          items.length > 0
            ? `${items.length} ${t("waiting for you")}`
            : undefined
        }
      />
      {profile && off && (
        <section className="xc-card recipes-discover-off">
          <Globe size={18} />
          <div>
            <strong>{t("Web search is off")}</strong>
            <p>
              {t(
                "Finding recipes online needs web search. Choose OpenAI built-in search or Brave Search in the AI settings.",
              )}
            </p>
          </div>
          <Link className="xc-btn small" to="/settings/assistant#ai-web-search">
            {t("Go to settings")}
          </Link>
        </section>
      )}
      {profile && !off && (
        <section className="xc-card recipes-discover">
          <div className="recipes-discover-profile">
            <span className="recipes-filter-label">{t("Your taste")}</span>
            {profile.tags.length === 0 && profile.nutrition.length === 0 ? (
              <span className="recipes-muted">
                {t("Not enough records yet. Searching by what you type.")}
              </span>
            ) : (
              <>
                {profile.tags.map((x) => (
                  <span key={x} className="xc-badge">
                    {x}
                  </span>
                ))}
                {profile.nutrition.map((n) => (
                  <span key={n} className={`xc-badge ${nutritionTone(n)}`}>
                    {t(NUTRITION_LABELS[n])}
                  </span>
                ))}
              </>
            )}
            {profile.dietGoal !== "none" && (
              <span className="recipes-muted">
                · {t("Diet goal")}：{t(GOAL_LABELS[profile.dietGoal])}
              </span>
            )}
          </div>
          <div className="recipes-discover-form">
            <input
              className="xc-input"
              value={hint}
              maxLength={100}
              aria-label={t("What do you feel like eating?")}
              placeholder={t(
                "What do you feel like eating? Optional, e.g. a light soup",
              )}
              onChange={(e) => setHint(e.target.value)}
              onKeyDown={(e) => {
                if (
                  e.key === "Enter" &&
                  !e.nativeEvent.isComposing &&
                  !discover.isPending &&
                  profile.runsLeft > 0
                )
                  run();
              }}
            />
            <select
              className="xc-select"
              aria-label={t("How many dishes")}
              value={count}
              onChange={(e) => setCount(Number(e.target.value))}
            >
              {[1, 2, 3, 4, 5].map((n) => (
                <option key={n} value={n}>
                  {n} {t("dishes")}
                </option>
              ))}
            </select>
            <button
              className="xc-btn primary"
              disabled={discover.isPending || profile.runsLeft <= 0}
              onClick={run}
            >
              <Search size={15} />
              {discover.isPending ? t("Searching…") : t("Find for me")}
            </button>
          </div>
          <small className="recipes-muted">
            {discover.isPending
              ? t("This can take about a minute.")
              : `${t("Searches left today")}：${profile.runsLeft}`}
          </small>
        </section>
      )}
      {items.length > 0 && (
        <p className="recipes-note">
          {t(
            "Recipes online can be wrong. Check the amounts against the source before you add one.",
          )}
        </p>
      )}
      {body}
    </div>
  );
}

function CandidateCard({ candidate: c }: { candidate: RecipeCandidate }) {
  const t = useT();
  const navigate = useNavigate();
  const update = useUpdateCandidate();
  const [open, setOpen] = useState(false);
  const d = c.draft;
  const ingredients = d.ingredients ?? [];
  const steps = d.steps ?? [];
  return (
    <article className="xc-card recipes-candidate">
      <div className="recipes-candidate-head">
        <strong>{d.title}</strong>
        {d.totalMinutes != null && (
          <span className="recipes-muted">
            {d.totalMinutes} {t("min")}
          </span>
        )}
      </div>
      {d.summary && <p className="recipes-card-summary">{d.summary}</p>}
      {c.reason && <p className="recipes-candidate-reason">{c.reason}</p>}
      <div className="recipes-card-tags">
        {(d.tags ?? []).slice(0, 5).map((x) => (
          <span key={x} className="xc-badge">
            {x}
          </span>
        ))}
        {(d.nutrition ?? []).map((n) => (
          <span key={n} className={`xc-badge ${nutritionTone(n)}`}>
            {t(NUTRITION_LABELS[n])}
          </span>
        ))}
      </div>
      <small className="recipes-card-meta">
        <span>
          {ingredients.length} {t("ingredients")}
        </span>
        <span>
          {steps.length} {t("steps")}
        </span>
        {d.sourceUrl && (
          <a href={d.sourceUrl} target="_blank" rel="noreferrer">
            <ExternalLink size={12} /> {hostOf(d.sourceUrl)}
          </a>
        )}
      </small>
      {open && (
        <div className="recipes-candidate-detail">
          <ul className="recipes-ingredients">
            {ingredients.map((ing, i) => (
              <li key={i}>
                <span>
                  {ing.name}
                  {ing.note && <small>{ing.note}</small>}
                </span>
                <span className="recipes-amount">{ing.amount}</span>
              </li>
            ))}
          </ul>
          <ol className="recipes-candidate-steps">
            {steps.map((s, i) => (
              <li key={i}>
                {s.title && <strong>{s.title}：</strong>}
                {s.text}
              </li>
            ))}
          </ol>
          {c.sources.length > 0 && (
            <div className="recipes-sources">
              <small className="recipes-muted">{t("Pages read")}</small>
              {c.sources.map((s) => (
                <a key={s.url} href={s.url} target="_blank" rel="noreferrer">
                  <ExternalLink size={12} /> {s.title || hostOf(s.url)}
                </a>
              ))}
            </div>
          )}
        </div>
      )}
      <div className="recipes-candidate-actions">
        <button
          className="xc-btn small ghost"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          {open ? t("Collapse") : t("Show details")}
        </button>
        <span className="recipes-candidate-spacer" />
        <button
          className="xc-btn small"
          disabled={update.isPending}
          onClick={() =>
            update.mutate(
              { id: c.id, status: "dismissed" },
              {
                onError: (err) =>
                  toast({ message: errorMessage(err), tone: "error" }),
              },
            )
          }
        >
          <X size={14} /> {t("Not this one")}
        </button>
        <button
          className="xc-btn small primary"
          onClick={() =>
            navigate("/recipes/new", {
              state: { draft: c.draft, candidateId: c.id },
            })
          }
        >
          <Plus size={14} /> {t("Add to menu")}
        </button>
      </div>
    </article>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
