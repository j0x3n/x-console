import { useSearchParams, Link } from "react-router";
import { Plus } from "lucide-react";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { Toolbar } from "../../components/ui/Toolbar";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import Markdown from "../../components/markdown/Markdown";
import { useT } from "../../contexts/LanguageContext";
import { isNotLive } from "../../api/client";
import { toast } from "../../hooks/useToast";
import { useHabitList, useUpdateHabit, useSchedule } from "./api";
import {
  averageWeight,
  dateKey,
  personalSession,
  personalWeek,
  personalWeekDates,
  validPersonalDate,
  planSections as sections,
} from "./personal";
import {
  useActivatePersonalHabits,
  useCheckPersonalHabit,
  usePersonalDay,
  usePersonalDays,
  usePersonalLibrary,
  usePersonalProfile,
  type LibraryHabit,
  type PersonalDay,
  type PersonalLibrary,
  type PersonalProfile,
} from "./personalApi";
import PersonalTraining from "./PersonalTraining";
import PersonalRecords, { PersonalNote } from "./PersonalRecords";
import PersonalSettings from "./PersonalSettings";
import PlanTimer from "./PlanTimer";
import RecommendPage from "./RecommendPage";
import { weekdayLabels } from "./workout";

/**
 * /habits/plan：默认是推荐习惯（用户 2026-10-05 要求“个人计划就当作推荐习惯页面”），
 * ?section= 打开个人计划的其他栏目。
 */
export default function PersonalPlanPage() {
  const [params] = useSearchParams();
  const section = params.get("section") ?? "recommend";
  if (section === "recommend" || !sections.some(([id]) => id === section))
    return <RecommendPage />;
  return <PlanSection section={section} />;
}

function PlanSection({ section }: { section: string }) {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const library = usePersonalLibrary();
  const profile = usePersonalProfile();
  const schedule = useSchedule();
  const today = dateKey(new Date(), schedule.data?.timezone);
  const rawDate = params.get("date") ?? today;
  const date = validPersonalDate(rawDate) ? rawDate : today;
  const day = usePersonalDay(date);
  const days = usePersonalDays();
  const go = (next: string, selectedDate = date) => {
    const p = new URLSearchParams(params);
    p.set("section", next);
    p.set("date", selectedDate);
    setParams(p);
  };
  if (
    library.isPending ||
    profile.isPending ||
    day.isPending ||
    schedule.isPending
  )
    return <Loading />;
  const error = library.error ?? profile.error ?? day.error ?? schedule.error;
  if (error)
    return isNotLive(error) ? (
      <NotLive name={t("Personal plan")} />
    ) : (
      <ErrorState
        error={error}
        onRetry={() => {
          void library.refetch();
          void profile.refetch();
          void day.refetch();
          void schedule.refetch();
        }}
      />
    );
  if (!library.data || !profile.data || !day.data)
    return <EmptyState title={t("No personal plan content")} />;
  const p = profile.data,
    d = day.data,
    data = library.data;
  const avg = averageWeight(days.data ?? [], date);
  const session = personalSession(date, p, data);
  const category = ["daily", "food", "english"].includes(section)
    ? section
    : "";
  return (
    <div className="xc-stack habits-personal">
      {section === "overview" && (
        <StatStrip label={t("Personal plan")}>
          <StatCard
            label={t("Strength phase")}
            value={data.phases.find((x) => x.id === p.phase)!.name}
            foot={`${t("Plan week")} ${personalWeek(date, p.start)}`}
          />
          <StatCard
            label={t("Selected plan habits")}
            value={Object.keys(p.habitIds).length}
            foot={`${Object.values(d.checks).filter(Boolean).length} ${t("completed on selected date")}`}
          />
          <StatCard
            label={t("Seven-day average")}
            value={avg === null ? "—" : avg.toFixed(1)}
            unit="kg"
            foot={t("Only recorded weights")}
          />
        </StatStrip>
      )}
      <Toolbar
        start={
          <h2 className="habits-personal-title">
            {t(sections.find(([id]) => id === section)![1])}
          </h2>
        }
        end={
          <label className="xc-row">
            <span>{t("Date")}</span>
            <input
              className="xc-input"
              type="date"
              aria-label={t("Plan date")}
              value={date}
              onChange={(e) => {
                if (validPersonalDate(e.target.value))
                  go(section, e.target.value);
              }}
            />
          </label>
        }
      />
      {(section === "overview" || section === "training") && (
        <WeekSchedule
          date={date}
          profile={p}
          library={data}
          onSelect={(v) => go("training", v)}
        />
      )}
      {section === "overview" && (
        <>
          <div className="habits-personal-grid">
            <section className="xc-card">
              <div className="xc-card-head">
                <h2>{session.name}</h2>
                <button className="xc-btn small" onClick={() => go("training")}>
                  {t("Open guided training")}
                </button>
              </div>
              <p className="xc-muted">
                {session.place} · {session.time}
              </p>
              <ul>
                {session.items.map((item, i) => (
                  <li key={i}>
                    {data.exercises.find((e) => e.id === item.exerciseId)!.name}{" "}
                    · {item.sets} {t("Sets")} × {item.prescription}
                  </li>
                ))}
              </ul>
            </section>
            <section className="xc-card">
              <div className="xc-card-head">
                <h2>{t("Daily rhythm")}</h2>
              </div>
              <p>
                <strong>{p.wake}</strong> {t("Wake time")}
              </p>
              <p>
                {t(
                  "Record weight, include protein in the first meal and learn new words when alert.",
                )}
              </p>
              <p>
                {t(
                  "Take eye and movement breaks during work. Spread your steps throughout the day.",
                )}
              </p>
              <p>
                <strong>{p.sleep}</strong> {t("Sleep time")}
              </p>
              <p>
                {t(
                  "Review words, record recovery and keep enough time for sleep.",
                )}
              </p>
            </section>
          </div>
          <PlanChecklist day={d} profile={p} items={data.habits} />
        </>
      )}
      {section === "training" && (
        <PersonalTraining key={date} day={d} profile={p} library={data} />
      )}
      {!!category && (
        <>
          <PlanChecklist
            day={d}
            profile={p}
            items={data.habits.filter((h) => h.category === category)}
          />
          {(category === "english" || category === "food") && (
            <PersonalNote
              key={`${date}:${category}`}
              day={d}
              field={category}
            />
          )}
          <PlanArticles library={data} category={category} />
        </>
      )}
      {section === "records" &&
        (days.isPending ? (
          <Loading />
        ) : days.isError ? (
          <ErrorState error={days.error} onRetry={() => days.refetch()} />
        ) : (
          <PersonalRecords
            key={date}
            day={d}
            days={days.data}
            onDate={(v) => go("records", v)}
          />
        ))}
      {section === "settings" && (
        <PersonalSettings key={p.start} profile={p} library={data} />
      )}
      {section === "reference" && (
        <>
          <PlanArticles library={data} category="reference" />
          <section className="xc-card habits-original">
            <div className="xc-card-head">
              <h2>{t("Complete original content")}</h2>
            </div>
            <details>
              <summary>{t("Original conversation")}</summary>
              <pre>{data.original.chat}</pre>
            </details>
            <details>
              <summary>{t("Original training attachment")}</summary>
              <pre>{data.original.attachment}</pre>
            </details>
          </section>
        </>
      )}
      {section === "training" && (
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Walk-run progression")}</h2>
          </div>
          <div className="xc-table-wrap">
            <table className="xc-table">
              <thead>
                <tr>
                  <th>{t("Level")}</th>
                  <th>{t("Content")}</th>
                  <th>{t("Duration (minutes)")}</th>
                </tr>
              </thead>
              <tbody>
                {data.runLevels.map((r) => (
                  <tr key={r.id}>
                    <td>{r.id}</td>
                    <td>
                      {r.name} · {r.rounds} {t("Sets")}
                    </td>
                    <td>{r.durationMinutes}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="xc-muted">
            {t(
              "Repeat a level until recovery is steady. Change the level in Plan settings.",
            )}
          </p>
        </section>
      )}
      <PlanTimer level={data.runLevels[p.runLevel]} />
    </div>
  );
}

function WeekSchedule({
  date,
  profile,
  library,
  onSelect,
}: {
  date: string;
  profile: PersonalProfile;
  library: PersonalLibrary;
  onSelect: (date: string) => void;
}) {
  const t = useT();
  return (
    <div
      className="habits-personal-week"
      aria-label={t("Weekly training schedule")}
    >
      {personalWeekDates(date).map((d, i) => {
        const s = personalSession(d, profile, library);
        return (
          <button
            className={`xc-card habits-week-choice${d === date ? " active" : ""}`}
            key={d}
            onClick={() => onSelect(d)}
            aria-pressed={d === date}
          >
            <strong>
              {t(weekdayLabels[i])} · {d.slice(5)}
            </strong>
            <small>{s.name}</small>
          </button>
        );
      })}
    </div>
  );
}

function PlanChecklist({
  day,
  profile,
  items,
}: {
  day: PersonalDay;
  profile: PersonalProfile;
  items: LibraryHabit[];
}) {
  const t = useT();
  const activate = useActivatePersonalHabits();
  const check = useCheckPersonalHabit(day.date);
  const native = useHabitList();
  const restore = useUpdateHabit();
  const pending =
    activate.isPending ||
    check.isPending ||
    native.isPending ||
    restore.isPending;
  const missing = items.filter((h) => !profile.habitIds[h.id]);
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Plan habit checklist")}</h2>
        {missing.length > 0 && (
          <button
            className="xc-btn small"
            disabled={pending}
            onClick={() =>
              activate.mutate(
                missing.map((h) => h.id),
                { onSuccess: () => toast(t("Template added")) },
              )
            }
          >
            <Plus size={14} /> {t("Add all listed habits")}
          </button>
        )}
      </div>
      <p className="xc-muted">
        {t(
          "These check-ins appear in Today and habit statistics. Unchecking only undoes the check-in made here.",
        )}
      </p>
      {native.isError && (
        <ErrorState error={native.error} onRetry={() => native.refetch()} />
      )}
      <div className="habits-personal-checklist">
        {items.map((h) => {
          const id = profile.habitIds[h.id],
            habit = native.data?.find((n) => n.id === id),
            archived = habit?.archived;
          return (
            <div className="habits-plan-habit" key={h.id}>
              {id && !archived ? (
                <label className="xc-check">
                  <input
                    type="checkbox"
                    checked={day.checks[h.id] ?? false}
                    aria-label={h.name}
                    disabled={pending || native.isError}
                    onChange={(e) =>
                      check.mutate({ id: h.id, done: e.target.checked })
                    }
                  />
                  <span>{h.name}</span>
                </label>
              ) : (
                <strong>{h.name}</strong>
              )}
              <small>
                {h.group} · {h.tip}
              </small>
              <div className="xc-row">
                {!id ? (
                  <button
                    className="xc-btn small"
                    disabled={pending || native.isError}
                    onClick={() =>
                      activate.mutate([h.id], {
                        onSuccess: () => toast(t("Template added")),
                      })
                    }
                  >
                    <Plus size={14} /> {t("Add as habit")}
                  </button>
                ) : archived ? (
                  <button
                    className="xc-btn small"
                    disabled={pending}
                    onClick={() =>
                      restore.mutate({ id, body: { archived: false } })
                    }
                  >
                    {t("Restore")}
                  </button>
                ) : (
                  <span className="xc-badge info">{t("Plan habit added")}</span>
                )}
                {h.exerciseId && (
                  <Link
                    className="xc-btn ghost small"
                    to={`/habits/fitness?exercise=${h.exerciseId}`}
                  >
                    {t("Exercise instructions")}
                  </Link>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function PlanArticles({
  library,
  category,
}: {
  library: PersonalLibrary;
  category: string;
}) {
  return (
    <div className="habits-personal-grid">
      {library.articles
        .filter((a) => a.category === category)
        .map((a) => (
          <section className="xc-card habits-plan-article" key={a.id}>
            <div className="xc-card-head">
              <h2>{a.title}</h2>
            </div>
            <Markdown source={a.content} />
          </section>
        ))}
    </div>
  );
}
