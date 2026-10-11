import { useSearchParams } from "react-router";
import { Layers, Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useCardSummary } from "./api";
import CardDialog from "./CardDialog";
import CardList from "./CardList";
import CardsSettingsTab from "./CardsSettingsTab";
import ReviewTab from "./ReviewTab";
import "./i18n";
import "./cards.css";

type Tab = "review" | "cards" | "settings";
const TABS: { id: Tab; label: string }[] = [
  { id: "review", label: "Review" },
  { id: "cards", label: "All cards" },
  { id: "settings", label: "Review settings" },
];

/** 复习卡片（B130）：今天要复习的卡、卡片列表和设置。 */
export default function CardsPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const summary = useCardSummary();
  const creating = params.get("new") === "1";
  const setCreating = (on: boolean) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (on) next.set("new", "1");
        else next.delete("new");
        return next;
      },
      { replace: true },
    );
  const param = params.get("tab");
  const tab: Tab = TABS.some((x) => x.id === param) ? (param as Tab) : "review";
  const setTab = (next: Tab) =>
    setParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (next === "review") p.delete("tab");
        else p.set("tab", next);
        return p;
      },
      { replace: true },
    );

  if (summary.isPending) return <Loading />;
  if (summary.isError && isNotLive(summary.error))
    return <NotLive name={t("Review cards")} icon={<Layers size={28} />} />;
  if (summary.isError)
    return (
      <ErrorState error={summary.error} onRetry={() => summary.refetch()} />
    );

  const s = summary.data;
  const done = s.reviewedToday + s.learnedToday;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Review cards")}
        subtitle={
          s.due > 0
            ? `${s.due} ${t("cards to review today")}`
            : t("Nothing to review right now")
        }
        aside={
          <button
            className="xc-btn primary"
            title={t("New card")}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} /> {t("New card")}
          </button>
        }
      />
      <StatStrip label={t("Review cards")}>
        <StatCard
          label={t("Cards due today")}
          value={s.due}
          tone={s.due > 0 ? "accent" : undefined}
          onClick={() => setTab("review")}
        />
        <StatCard label={t("Reviewed today")} value={done} />
        <StatCard
          label={t("All cards")}
          value={s.total}
          foot={s.suspended > 0 ? `${s.suspended} ${t("paused")}` : undefined}
          onClick={() => setTab("cards")}
        />
        <StatCard
          label={t("Remembered, last 30 days")}
          value={s.recall30 == null ? "–" : `${Math.round(s.recall30 * 100)}%`}
        />
      </StatStrip>
      <div className="xc-tabs" role="tablist">
        {TABS.map((x) => (
          <button
            key={x.id}
            role="tab"
            aria-selected={tab === x.id}
            className={tab === x.id ? "active" : ""}
            onClick={() => setTab(x.id)}
          >
            {t(x.label)}
          </button>
        ))}
      </div>
      {tab === "review" && (
        <ReviewTab summary={s} onNew={() => setCreating(true)} />
      )}
      {tab === "cards" && <CardList onNew={() => setCreating(true)} />}
      {tab === "settings" && <CardsSettingsTab />}
      {creating && <CardDialog onClose={() => setCreating(false)} />}
    </div>
  );
}
