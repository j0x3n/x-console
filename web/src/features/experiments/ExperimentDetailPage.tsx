import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, Pencil, Square, Trash2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  toInput,
  useDeleteExperiment,
  useExperiment,
  useSaveExperiment,
  type Experiment,
  type ExperimentResult,
} from "./api";
import ExperimentDialog from "./ExperimentDialog";
import {
  CHART_H,
  CHART_W,
  NOTE_TEXT,
  PHASE_LABELS,
  PHASE_TONE,
  VERDICT_TONE,
  fmt,
  scatter,
  signed,
  verdictText,
} from "./format";
import "./i18n";
import "./experiments.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 一个实验：设置、每个指标的结果、结论。 */
export default function ExperimentDetailPage() {
  const t = useT();
  const navigate = useNavigate();
  const id = Number(useParams().id);
  const q = useExperiment(id);
  const save = useSaveExperiment();
  const remove = useDeleteExperiment();
  const [editing, setEditing] = useState(false);

  if (q.isPending) return <Loading />;
  if (q.isError && isNotLive(q.error))
    return <NotLive name={t("Experiments")} />;
  if (q.isError)
    return <ErrorState error={q.error} onRetry={() => q.refetch()} />;
  const { experiment: e, results } = q.data;

  const stop = () =>
    save.mutate(
      { id: e.id, body: { ...toInput(e), stopped: !e.stopped } },
      { onError: fail },
    );
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete experiment")} ${e.title}？`,
      description: t(
        "The experiment and its notes are deleted. The data it read stays.",
      ),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(e.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        navigate("/experiments");
      },
      onError: fail,
    });
  };

  return (
    <div className="xc-page experiments-page">
      <PageHeading
        title={e.title}
        aside={
          <>
            <button className="xc-btn small" onClick={() => setEditing(true)}>
              <Pencil size={14} /> {t("Edit experiment")}
            </button>
            <button className="xc-btn small" onClick={stop}>
              <Square size={14} />{" "}
              {e.stopped ? t("Resume experiment") : t("Stop experiment")}
            </button>
            <button className="xc-btn small" onClick={onDelete}>
              <Trash2 size={14} /> {t("Delete experiment")}
            </button>
          </>
        }
      />
      <Link to="/experiments" className="experiments-back">
        <ArrowLeft size={14} /> {t("All experiments")}
      </Link>
      <section className="xc-card experiments-summary">
        <div className="experiments-item-head">
          <strong>{e.change || e.title}</strong>
          <span className={`xc-badge ${PHASE_TONE[e.phase]}`}>
            {t(PHASE_LABELS[e.phase])}
          </span>
        </div>
        <dl className="experiments-dates-list">
          <div>
            <dt>{t("Baseline starts")}</dt>
            <dd>{e.baselineStart}</dd>
          </div>
          <div>
            <dt>{t("Change starts")}</dt>
            <dd>{e.changeStart}</dd>
          </div>
          <div>
            <dt>{t("Experiment ends")}</dt>
            <dd>{e.endDate}</dd>
          </div>
        </dl>
        {e.note && <p className="xc-muted">{e.note}</p>}
        <p className="xc-muted experiments-caution">
          {t(
            "This compares two stretches of days, not randomly assigned groups. Season, workload and mood in the same weeks also move the numbers.",
          )}
        </p>
      </section>
      <div className="experiments-results">
        {results.map((r) => (
          <ResultCard key={r.metric.key} result={r} experiment={e} />
        ))}
      </div>
      <Conclusion experiment={e} />
      <ExperimentDialog
        open={editing}
        onClose={() => setEditing(false)}
        experiment={e}
      />
    </div>
  );
}

function Conclusion({ experiment }: { experiment: Experiment }) {
  const t = useT();
  const save = useSaveExperiment();
  const [text, setText] = useState(experiment.conclusion);
  useEffect(() => setText(experiment.conclusion), [experiment.conclusion]);
  return (
    <section className="xc-card" aria-label={t("Your conclusion")}>
      <div className="xc-card-head">
        <h2>{t("Your conclusion")}</h2>
      </div>
      <textarea
        className="xc-textarea"
        rows={3}
        value={text}
        maxLength={5000}
        aria-label={t("Your conclusion")}
        onChange={(ev) => setText(ev.target.value)}
        placeholder="看完结果，你打算怎么办"
      />
      <div className="experiments-actions">
        <button
          className="xc-btn small primary"
          disabled={save.isPending || text === experiment.conclusion}
          onClick={() =>
            save.mutate(
              {
                id: experiment.id,
                body: { ...toInput(experiment), conclusion: text },
              },
              { onSuccess: () => toast(t("Saved")), onError: fail },
            )
          }
        >
          {t("Save")}
        </button>
      </div>
    </section>
  );
}

function ResultCard({
  result: r,
  experiment: e,
}: {
  result: ExperimentResult;
  experiment: Experiment;
}) {
  const t = useT();
  const last = e.endDate;
  const s = scatter(r.points, e.baselineStart, last);
  const split = s.x(e.changeStart);
  const unit = r.metric.unit;
  return (
    <article className="xc-card experiments-result" aria-label={r.metric.label}>
      <div className="experiments-item-head">
        <strong>{r.metric.label}</strong>
        <span className={`xc-badge ${VERDICT_TONE[r.verdict]}`}>
          {verdictText(r.verdict, r)}
        </span>
      </div>
      <table className="experiments-table">
        <thead>
          <tr>
            <th />
            <th>{t("Days with data")}</th>
            <th>{t("Average")}</th>
            <th>{t("Spread (sd)")}</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th>{t("Baseline phase")}</th>
            <td>
              {r.baseline.n} / {r.baseline.days}
            </td>
            <td>
              {fmt(r.baseline.mean)} {unit}
            </td>
            <td>{fmt(r.baseline.sd)}</td>
          </tr>
          <tr>
            <th>{t("Change phase")}</th>
            <td>
              {r.change.n} / {r.change.days}
            </td>
            <td>
              {fmt(r.change.mean)} {unit}
            </td>
            <td>{fmt(r.change.sd)}</td>
          </tr>
        </tbody>
      </table>
      {r.diff != null && (
        <p className="experiments-diff">
          {t("Difference")}：{signed(r.diff)} {unit}
          {r.ciLow != null && r.ciHigh != null && (
            <small className="xc-muted">
              {" "}
              {`（95% 区间 ${fmt(r.ciLow)} 到 ${fmt(r.ciHigh)}）`}
            </small>
          )}
        </p>
      )}
      {r.points.length > 0 && (
        <svg
          viewBox={`0 0 ${CHART_W} ${CHART_H}`}
          className="experiments-chart"
          role="img"
          aria-label={`${r.metric.label} ${t("Daily values")}`}
        >
          <rect
            x={split}
            y={0}
            width={Math.max(CHART_W - split, 0)}
            height={CHART_H}
            className="experiments-change-bg"
          />
          {r.points.map((p) => (
            <circle
              key={p.date}
              cx={s.x(p.date)}
              cy={s.y(p.value)}
              r={3.5}
              className={
                p.phase === "change"
                  ? "experiments-dot change"
                  : "experiments-dot"
              }
            >
              <title>{`${p.date}: ${fmt(p.value)}`}</title>
            </circle>
          ))}
        </svg>
      )}
      {r.notes.length > 0 && (
        <ul className="experiments-notes">
          {r.notes.map((n) => (
            <li key={n}>{NOTE_TEXT[n]}</li>
          ))}
        </ul>
      )}
    </article>
  );
}
