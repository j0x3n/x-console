import { NavLink, useParams, useSearchParams } from "react-router";
import { Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import StatsView from "./StatsView";
import TodayView from "./TodayView";
import WorkoutView from "./WorkoutView";

const tabs = [
  { id: "", label: "Today", to: "/habits" },
  { id: "stats", label: "Stats", to: "/habits/stats" },
  { id: "workout", label: "Workout", to: "/habits/workout" },
];

export default function HabitsPage() {
  const t = useT();
  const { tab = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const current = tabs.find((x) => x.id === tab) ?? tabs[0];
  const creating = params.get("new") === "1";
  const setCreating = (open: boolean) => {
    const next = new URLSearchParams(params);
    if (open) next.set("new", "1");
    else next.delete("new");
    setParams(next, { replace: true });
  };
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Habits")}
        aside={
          current.id === "" && (
            <button
              className="xc-btn primary"
              onClick={() => setCreating(true)}
            >
              <Plus size={15} /> {t("New habit")}
            </button>
          )
        }
      />
      <nav className="xc-tabs">
        {tabs.map((item) => (
          <NavLink
            key={item.id}
            to={item.to}
            end
            className={item.id === current.id ? "active" : ""}
          >
            {t(item.label)}
          </NavLink>
        ))}
      </nav>
      {current.id === "" && (
        <TodayView
          creating={creating}
          onCloseCreate={() => setCreating(false)}
        />
      )}
      {current.id === "stats" && <StatsView />}
      {current.id === "workout" && <WorkoutView />}
    </div>
  );
}
