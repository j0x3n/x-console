import { useState } from "react";
import { useSearchParams } from "react-router";
import { Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import Markdown from "../../components/markdown/Markdown";
import { useT } from "../../contexts/LanguageContext";
import { usePersonalLibrary, type LibraryExercise } from "./personalApi";

export function ExerciseDetail({ exercise }: { exercise: LibraryExercise }) {
  const t = useT();
  return (
    <section className="xc-card habits-exercise-detail">
      <div className="xc-card-head">
        <h2>{exercise.name}</h2>
        <span className="xc-muted">{exercise.en}</span>
      </div>
      <p className="xc-muted">
        {exercise.group} · {exercise.equipment} · {exercise.target}
      </p>
      <div className="habits-exercise-dose">
        <span>
          {t("Reference dose")}：{exercise.dose}
        </span>
        <span>
          {t("Rest between sets")}：{exercise.rest}
        </span>
      </div>
      <Markdown source={`![${exercise.name}](${exercise.image})`} />
      <small className="xc-muted">
        {t("Illustrations support the written instructions. Click to enlarge.")}
      </small>
      <h3>{t("Exercise steps")}</h3>
      <ol>
        {exercise.steps.map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ol>
      <details>
        <summary>{t("Exercise setup")}</summary>
        <ul>
          {exercise.setup.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ul>
      </details>
      <h3>{t("Common mistakes")}</h3>
      <ul>
        {exercise.errors.map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ul>
      <h3>{t("Easier variation")}</h3>
      <p>{exercise.easier}</p>
      <h3>{t("When to progress")}</h3>
      <p>{exercise.progress}</p>
    </section>
  );
}

export function ExerciseLibrary({
  onSelect,
}: {
  onSelect?: (exercise: LibraryExercise) => void;
}) {
  const t = useT();
  const library = usePersonalLibrary();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const [group, setGroup] = useState("");
  const [selected, setSelected] = useState(params.get("exercise") ?? "squat");
  if (library.isPending) return <Loading />;
  if (library.isError)
    return isNotLive(library.error) ? (
      <NotLive name={t("Exercise library")} />
    ) : (
      <ErrorState error={library.error} onRetry={() => library.refetch()} />
    );
  const items = library.data.exercises.filter(
    (e) =>
      (!group || e.group === group) &&
      `${e.name} ${e.en} ${e.target} ${e.equipment}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
  );
  const active = items.find((e) => e.id === selected) ?? items[0];
  const groups = [...new Set(library.data.exercises.map((e) => e.group))];
  const select = (id: string) => {
    setSelected(id);
    if (!onSelect) {
      const next = new URLSearchParams(params);
      next.set("exercise", id);
      setParams(next, { replace: true });
    }
  };
  return (
    <div className="habits-library">
      <Toolbar
        start={
          <label className="xc-row">
            <span>{t("Exercise group")}</span>
            <select
              className="xc-select"
              aria-label={t("Exercise group")}
              value={group}
              onChange={(e) => setGroup(e.target.value)}
            >
              <option value="">{t("All")}</option>
              {groups.map((g) => (
                <option key={g}>{g}</option>
              ))}
            </select>
            <span className="xc-muted">
              {items.length} / {library.data.exercises.length}
            </span>
          </label>
        }
        end={
          <SearchBox
            value={search}
            onChange={setSearch}
            placeholder={t("Search exercises, muscles or equipment")}
          />
        }
      />
      {active ? (
        <div className="habits-library-layout">
          <aside className="xc-card habits-library-index">
            <div className="xc-card-head">
              <h2>{t("Exercise library")}</h2>
            </div>
            <select
              className="xc-select habits-library-mobile"
              aria-label={t("Choose exercise")}
              value={active.id}
              onChange={(e) => select(e.target.value)}
            >
              {items.map((e) => (
                <option value={e.id} key={e.id}>
                  {e.name}
                </option>
              ))}
            </select>
            <div className="habits-library-list">
              {items.map((e) => (
                <button
                  key={e.id}
                  className={`habits-library-item${e.id === active.id ? " active" : ""}`}
                  onClick={() => select(e.id)}
                  aria-pressed={e.id === active.id}
                >
                  <strong>{e.name}</strong>
                  <small>{e.en}</small>
                </button>
              ))}
            </div>
          </aside>
          <div className="habits-library-main">
            {onSelect && (
              <div className="habits-library-select">
                <button
                  className="xc-btn primary"
                  onClick={() => onSelect(active)}
                  title={t("Select exercise")}
                >
                  <Plus size={15} /> {t("Select exercise")}：{active.name}
                </button>
              </div>
            )}
            <ExerciseDetail exercise={active} />
          </div>
        </div>
      ) : (
        <EmptyState title={t("No matching exercises")}>
          <button
            className="xc-btn"
            onClick={() => {
              setSearch("");
              setGroup("");
            }}
          >
            {t("Clear filters")}
          </button>
        </EmptyState>
      )}
    </div>
  );
}

export function ExercisePicker({
  open,
  onClose,
  onSelect,
}: {
  open: boolean;
  onClose: () => void;
  onSelect: (exercise: LibraryExercise) => void;
}) {
  const t = useT();
  return (
    <Dialog open={open} onClose={onClose} title={t("Choose exercise")} wide>
      {open && (
        <ExerciseLibrary
          onSelect={(e) => {
            onSelect(e);
            onClose();
          }}
        />
      )}
    </Dialog>
  );
}
