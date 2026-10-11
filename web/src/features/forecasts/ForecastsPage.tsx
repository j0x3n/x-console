import { useState, type FormEvent } from "react";
import { Scale, Settings } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateForecast,
  useDeleteForecast,
  useForecasts,
  useForecastSettings,
  useForecastStats,
  useReopenForecast,
  useResolveForecast,
  type Forecast,
  type ForecastKind,
  type ForecastOutcome,
} from "./api";
import { biasText, OUTCOME_LABEL, pct } from "./format";
import ForecastSettingsDialog from "./ForecastSettingsDialog";
import "./i18n";
import "./forecasts.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

const OUTCOMES: ForecastOutcome[] = ["yes", "no", "void"];

/** 预测和决策记录（B137）：写下判断和把握，到期填结果，看自己偏高还是偏低。 */
export default function ForecastsPage() {
  const t = useT();
  const list = useForecasts();
  const stats = useForecastStats();
  const settings = useForecastSettings();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const items = list.data?.items ?? [];
  const open = items.filter((f) => !f.outcome);
  const done = items.filter((f) => f.outcome);
  const overdue = open.filter((f) => f.overdue).length;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Forecasts")} icon={<Scale size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else
    body = (
      <>
        <section className="fc-section">
          <h2 className="fc-title">{t("Forecast waiting")}</h2>
          {open.length === 0 ? (
            <EmptyState
              title={t("Forecast waiting empty")}
              icon={<Scale size={28} />}
            />
          ) : (
            <div className="fc-list">
              {open.map((f) => (
                <OpenRow key={f.id} item={f} />
              ))}
            </div>
          )}
        </section>
        <StatsCard stats={stats.data} />
        <details className="fc-done">
          <summary>
            {t("Forecast done")} ({done.length})
          </summary>
          {done.length === 0 ? (
            <p className="xc-muted">{t("Forecast done empty")}</p>
          ) : (
            <div className="fc-list">
              {done.map((f) => (
                <DoneRow key={f.id} item={f} />
              ))}
            </div>
          )}
        </details>
      </>
    );

  return (
    <div className="xc-page fc-page">
      <PageHeading
        title={t("Forecasts")}
        subtitle={
          open.length > 0
            ? `${open.length} ${t("Forecast open count")}`
            : undefined
        }
        aside={
          <button
            className="xc-btn small"
            onClick={() => setSettingsOpen(true)}
          >
            <Settings size={14} /> {t("Forecast settings")}
          </button>
        }
      />
      {overdue > 0 && (
        <StatStrip label={t("Forecasts")}>
          <StatCard
            label={t("Forecast overdue")}
            value={overdue}
            unit={t("Forecast overdue count")}
            tone="danger"
          />
        </StatStrip>
      )}
      <AddForm today={list.data?.today} />
      {body}
      <ForecastSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings.data}
      />
    </div>
  );
}

function AddForm({ today }: { today?: string }) {
  const t = useT();
  const create = useCreateForecast();
  const [kind, setKind] = useState<ForecastKind>("prediction");
  const [statement, setStatement] = useState("");
  const [probability, setProbability] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [reason, setReason] = useState("");

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const p = probability.trim() === "" ? undefined : Number(probability);
    create.mutate(
      {
        kind,
        statement: statement.trim(),
        probability: p,
        dueDate,
        reason: reason.trim() || undefined,
      },
      {
        onSuccess: () => {
          setStatement("");
          setProbability("");
          setDueDate("");
          setReason("");
        },
        onError: fail,
      },
    );
  };

  return (
    <form className="xc-card fc-form" onSubmit={submit}>
      <div className="fc-kinds" role="group" aria-label={t("Forecast kind")}>
        {(["prediction", "decision"] as const).map((k) => (
          <button
            key={k}
            type="button"
            className={`xc-btn small${kind === k ? " primary" : ""}`}
            aria-pressed={kind === k}
            onClick={() => setKind(k)}
          >
            {t(
              k === "prediction"
                ? "Forecast kind prediction"
                : "Forecast kind decision",
            )}
          </button>
        ))}
      </div>
      <label className="xc-field fc-wide">
        <span>{t("Forecast statement")}</span>
        <input
          className="xc-input"
          value={statement}
          maxLength={500}
          placeholder={t(
            kind === "prediction"
              ? "Forecast statement prediction hint"
              : "Forecast statement decision hint",
          )}
          onChange={(e) => setStatement(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Forecast confidence")}</span>
        <input
          className="xc-input"
          type="number"
          min={1}
          max={99}
          value={probability}
          placeholder={t("Forecast confidence hint")}
          onChange={(e) => setProbability(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Forecast due date")}</span>
        <input
          className="xc-input"
          type="date"
          min={today}
          value={dueDate}
          onChange={(e) => setDueDate(e.target.value)}
        />
      </label>
      <label className="xc-field fc-wide">
        <span>{t("Forecast reason")}</span>
        <textarea
          className="xc-textarea"
          rows={2}
          value={reason}
          maxLength={2000}
          placeholder={t("Forecast reason hint")}
          onChange={(e) => setReason(e.target.value)}
        />
      </label>
      <div className="fc-actions fc-wide">
        <button
          type="submit"
          className="xc-btn small primary"
          disabled={create.isPending || !statement.trim() || !dueDate}
        >
          {t("Forecast add")}
        </button>
      </div>
    </form>
  );
}

function Meta({ item }: { item: Forecast }) {
  const t = useT();
  return (
    <div className="fc-meta xc-muted">
      <span className="xc-badge">
        {t(
          item.kind === "prediction"
            ? "Forecast kind prediction"
            : "Forecast kind decision",
        )}
      </span>
      {item.probability != null && (
        <span>
          {t("Forecast confidence short")} {item.probability}%
        </span>
      )}
      <span>
        {t("Forecast due on")} {item.dueDate}
      </span>
      {item.overdue && (
        <span className="xc-badge danger">{t("Forecast overdue")}</span>
      )}
    </div>
  );
}

function OpenRow({ item }: { item: Forecast }) {
  const t = useT();
  const resolve = useResolveForecast();
  const remove = useDeleteForecast();
  const [note, setNote] = useState("");

  const answer = (outcome: ForecastOutcome) =>
    resolve.mutate(
      { id: item.id, outcome, note: note.trim() || undefined },
      { onError: fail },
    );
  const del = async () => {
    const ok = await confirmAction({
      title: t("Forecast delete title"),
      description: t("Forecast delete body"),
    });
    if (ok) remove.mutate(item.id, { onError: fail });
  };

  return (
    <article className={`xc-card fc-row${item.overdue ? " overdue" : ""}`}>
      <p className="fc-statement">{item.statement}</p>
      <Meta item={item} />
      {item.reason && <p className="fc-reason xc-muted">{item.reason}</p>}
      <input
        className="xc-input fc-note"
        value={note}
        maxLength={1000}
        aria-label={t("Forecast result note")}
        placeholder={t("Forecast result note")}
        onChange={(e) => setNote(e.target.value)}
      />
      <div className="fc-actions">
        {OUTCOMES.map((o) => (
          <button
            key={o}
            className="xc-btn small"
            disabled={resolve.isPending}
            onClick={() => answer(o)}
          >
            {t(OUTCOME_LABEL[item.kind][o])}
          </button>
        ))}
        <button className="xc-btn small ghost danger" onClick={del}>
          {t("Delete")}
        </button>
      </div>
    </article>
  );
}

function DoneRow({ item }: { item: Forecast }) {
  const t = useT();
  const reopen = useReopenForecast();
  const remove = useDeleteForecast();
  const outcome = item.outcome ?? "void";
  const del = async () => {
    const ok = await confirmAction({
      title: t("Forecast delete title"),
      description: t("Forecast delete body"),
    });
    if (ok) remove.mutate(item.id, { onError: fail });
  };
  return (
    <article className="xc-card fc-row">
      <p className="fc-statement">{item.statement}</p>
      <Meta item={item} />
      <div className="fc-meta">
        <span
          className={`xc-badge ${outcome === "yes" ? "ok" : outcome === "no" ? "danger" : ""}`}
        >
          {t(OUTCOME_LABEL[item.kind][outcome])}
        </span>
        {item.note && <span className="xc-muted">{item.note}</span>}
      </div>
      <div className="fc-actions">
        <button
          className="xc-btn small"
          disabled={reopen.isPending}
          onClick={() => reopen.mutate(item.id, { onError: fail })}
        >
          {t("Forecast undo outcome")}
        </button>
        <button className="xc-btn small ghost danger" onClick={del}>
          {t("Delete")}
        </button>
      </div>
    </article>
  );
}

function StatsCard({ stats }: { stats?: import("./api").ForecastStats }) {
  const t = useT();
  if (!stats) return null;
  return (
    <section className="xc-card fc-stats">
      <div className="xc-card-head">{t("Forecast stats")}</div>
      <p className="xc-muted fc-hint">{t("Forecast stats rule")}</p>
      {stats.count === 0 ? (
        <p className="xc-muted">{t("Forecast stats empty")}</p>
      ) : (
        <>
          <StatStrip label={t("Forecast stats")}>
            <StatCard label={t("Forecast stats counted")} value={stats.count} />
            <StatCard
              label={t("Forecast brier")}
              value={stats.brier.toFixed(3)}
              foot={t("Forecast brier hint")}
            />
            <StatCard
              label={t("Forecast bias")}
              value={`${stats.bias > 0 ? "+" : ""}${stats.bias.toFixed(1)}`}
              unit="%"
              foot={t(`Forecast bias ${biasText(stats.bias)}`)}
            />
          </StatStrip>
          {stats.small && (
            <p className="fc-small">{t("Forecast small sample")}</p>
          )}
          <table className="fc-bins">
            <thead>
              <tr>
                <th>{t("Forecast bin")}</th>
                <th>{t("Forecast bin count")}</th>
                <th>{t("Forecast bin said")}</th>
                <th>{t("Forecast bin happened")}</th>
              </tr>
            </thead>
            <tbody>
              {stats.bins.map((b) => (
                <tr key={b.label} className={b.count === 0 ? "empty" : ""}>
                  <td>{b.label}</td>
                  <td>
                    {b.count}
                    {b.count > 0 && b.few && (
                      <small className="xc-muted">
                        {" "}
                        {t("Forecast bin few")}
                      </small>
                    )}
                  </td>
                  <td>
                    {b.count > 0 ? `${b.meanProbability.toFixed(0)}%` : "–"}
                  </td>
                  <td>
                    {b.count > 0 ? (
                      <span className="fc-bar" aria-label={`${b.observed}%`}>
                        <span style={{ width: pct(b.observed) }} />
                        <em>{b.observed.toFixed(0)}%</em>
                      </span>
                    ) : (
                      "–"
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
      <p className="xc-muted fc-hint">{t("Forecast evidence")}</p>
    </section>
  );
}
