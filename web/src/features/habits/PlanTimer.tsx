import { useEffect, useState } from "react";
import { Pause, Play, X } from "lucide-react";
import { toast } from "../../hooks/useToast";
import { useT } from "../../contexts/LanguageContext";
import type { PersonalLibrary } from "./personalApi";

type Part = { name: string; seconds: number };
export default function PlanTimer({
  level,
}: {
  level: PersonalLibrary["runLevels"][number];
}) {
  const t = useT();
  const [parts, setParts] = useState<Part[]>([]);
  const [index, setIndex] = useState(0);
  const [remaining, setRemaining] = useState(0);
  const [deadline, setDeadline] = useState<number | null>(null);
  const start = (next: Part[]) => {
    setParts(next);
    setIndex(0);
    setRemaining(next[0].seconds);
    setDeadline(Date.now() + next[0].seconds * 1000);
  };
  useEffect(() => {
    if (deadline === null) return;
    const tick = () => {
      const left = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      setRemaining(left);
      if (!left) {
        setDeadline(null);
        toast(t("Timer finished"));
      }
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [deadline, t]);
  const walkRun = () => {
    const next: Part[] = [];
    if (!level.id)
      next.push({ name: level.name, seconds: level.durationMinutes * 60 });
    else
      for (let i = 0; i < level.rounds; i++) {
        if (level.walkMinutes)
          next.push({
            name: `${t("Walk")} ${i + 1}/${level.rounds}`,
            seconds: level.walkMinutes * 60,
          });
        if (level.runMinutes)
          next.push({
            name: `${t("Easy run")} ${i + 1}/${level.rounds}`,
            seconds: level.runMinutes * 60,
          });
      }
    next.push({ name: t("Cool down walk"), seconds: 300 });
    start(next);
  };
  const nextPart = () => {
    const n = index + 1;
    setIndex(n);
    setRemaining(parts[n].seconds);
    setDeadline(Date.now() + parts[n].seconds * 1000);
  };
  return (
    <section className="xc-card habits-plan-timer">
      <div className="xc-card-head">
        <h2>{t("Plan timer")}</h2>
        <span className="xc-muted">
          {t("Keep this page open. Background tabs may delay the prompt.")}
        </span>
      </div>
      <div className="xc-row">
        {[
          ["Set rest", 90],
          ["Eye break", 1200],
          ["English study", 1200],
        ].map(([name, seconds]) => (
          <button
            className="xc-btn small"
            key={name}
            onClick={() =>
              start([{ name: t(String(name)), seconds: Number(seconds) }])
            }
          >
            <Play size={14} /> {t(String(name))}
          </button>
        ))}
        <button className="xc-btn small" onClick={walkRun}>
          <Play size={14} /> {t("Walk-run intervals")}
        </button>
      </div>
      {!!parts.length && (
        <div className="habits-timer-active" role="status">
          <strong>
            {parts[index].name} ·{" "}
            {String(Math.floor(remaining / 60)).padStart(2, "0")}:
            {String(remaining % 60).padStart(2, "0")}
          </strong>
          {remaining > 0 ? (
            <button
              className="xc-btn small"
              title={t(deadline === null ? "Resume" : "Pause")}
              onClick={() =>
                setDeadline(
                  deadline === null ? Date.now() + remaining * 1000 : null,
                )
              }
            >
              {deadline === null ? <Play size={14} /> : <Pause size={14} />}
              {t(deadline === null ? "Resume" : "Pause")}
            </button>
          ) : index < parts.length - 1 ? (
            <button className="xc-btn small" onClick={nextPart}>
              {t("Next interval")}
            </button>
          ) : (
            <span className="xc-badge ok">{t("Finished")}</span>
          )}
          <button
            className="xc-btn ghost small"
            title={t("Close timer")}
            onClick={() => {
              setParts([]);
              setDeadline(null);
            }}
          >
            <X size={14} />
          </button>
        </div>
      )}
    </section>
  );
}
