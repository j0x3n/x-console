import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import Switch from "../../components/ui/Switch";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useAnswerCheckin,
  useCheckinQuestions,
  useCheckinSeries,
  useCheckinSettings,
  useCheckinsToday,
  useDeleteCheckinQuestion,
  usePutCheckinSettings,
  type CheckinQuestion,
  type CheckinSeries,
  type CheckinSlot,
} from "./api";
import { CHART_H, CHART_W, fmtScore, linePoints, pathOf } from "./chart";
import QuestionDialog from "./QuestionDialog";
import "./i18n";
import "./checkins.css";

const RANGES = [7, 30, 90] as const;
const STATUS_TONE: Record<CheckinSlot["status"], string> = {
  answered: "ok",
  pending: "accent",
  upcoming: "",
  missed: "warn",
};
const STATUS_LABEL: Record<CheckinSlot["status"], string> = {
  answered: "Slot answered",
  pending: "Check-in waiting",
  upcoming: "Slot not yet",
  missed: "Slot missed",
};

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 每天状态打卡（B132）：今天的问题、最近的平均分、问题管理。 */
export default function CheckinsPage() {
  const t = useT();
  const today = useCheckinsToday();
  const settings = useCheckinSettings();
  const setEnabled = usePutCheckinSettings();
  const questions = useCheckinQuestions();
  const [days, setDays] = useState<number>(30);
  const series = useCheckinSeries(days);
  const [editing, setEditing] = useState<CheckinQuestion | "new" | null>(null);

  if (today.isPending) return <Loading />;
  if (today.isError && isNotLive(today.error))
    return <NotLive name={t("Check-ins")} />;
  if (today.isError)
    return <ErrorState error={today.error} onRetry={() => today.refetch()} />;

  const slots = today.data.slots;
  const waiting = slots.filter(
    (s) => s.status === "pending" || s.status === "missed",
  ).length;
  return (
    <div className="xc-page checkins-page">
      <PageHeading
        title={t("Check-ins")}
        subtitle={waiting > 0 ? `${waiting} ${t("to answer")}` : undefined}
        aside={
          <button
            className="xc-btn small primary"
            onClick={() => setEditing("new")}
          >
            <Plus size={14} /> {t("New question")}
          </button>
        }
      />
      <section className="xc-card checkins-ask">
        <div className="checkins-ask-row">
          <div>
            <strong>{t("Ask me every day")}</strong>
            <p className="xc-muted">
              {t(
                "At the times below a notification asks for a score from 1 to 5. In Telegram you answer with one tap. The scores are only kept so you can compare days later.",
              )}
            </p>
          </div>
          <Switch
            checked={settings.data?.enabled ?? false}
            disabled={settings.isPending || setEnabled.isPending}
            label={t("Ask me every day")}
            onChange={(v) => setEnabled.mutate(v, { onError: fail })}
          />
        </div>
      </section>

      <section className="xc-card" aria-label={t("Today")}>
        <div className="xc-card-head">
          <h2>{t("Today")}</h2>
        </div>
        {slots.length === 0 ? (
          <p className="xc-muted">{t("No question is on for today.")}</p>
        ) : (
          <ul className="checkins-slots">
            {slots.map((s) => (
              <SlotRow key={`${s.questionId}-${s.slot}`} slot={s} />
            ))}
          </ul>
        )}
      </section>

      <section className="checkins-trend" aria-label={t("Trend")}>
        <div className="checkins-trend-head">
          <h2>{t("Trend")}</h2>
          <Segmented<string>
            label={t("Time range")}
            value={String(days)}
            onChange={(v) => setDays(Number(v))}
            options={RANGES.map((n) => ({
              value: String(n),
              label: `${n} ${t("days")}`,
            }))}
          />
        </div>
        {series.isPending ? (
          <Loading />
        ) : series.isError ? (
          <ErrorState error={series.error} onRetry={() => series.refetch()} />
        ) : (
          <div className="checkins-trend-grid">
            {series.data.items.map((s) => (
              <TrendCard
                key={s.questionId}
                item={s}
                from={series.data.from}
                to={series.data.to}
              />
            ))}
          </div>
        )}
      </section>

      <section className="xc-card" aria-label={t("Questions")}>
        <div className="xc-card-head">
          <h2>{t("Questions")}</h2>
        </div>
        <QuestionList items={questions.data?.items ?? []} onEdit={setEditing} />
      </section>
      <QuestionDialog
        open={editing !== null}
        question={editing === "new" ? null : editing}
        onClose={() => setEditing(null)}
      />
    </div>
  );
}

function SlotRow({ slot }: { slot: CheckinSlot }) {
  const t = useT();
  const answer = useAnswerCheckin();
  const pick = (value: number) =>
    answer.mutate(
      { questionId: slot.questionId, slot: slot.slot, value },
      { onError: fail },
    );
  return (
    <li className="checkins-slot">
      <div className="checkins-slot-main">
        <strong>{slot.label}</strong>
        <small className="xc-muted">
          {slot.slot} · {t("Score 1")}：{slot.lowLabel || "—"} · {t("Score 5")}
          ：{slot.highLabel || "—"}
        </small>
      </div>
      <span className={`xc-badge ${STATUS_TONE[slot.status]}`}>
        {t(STATUS_LABEL[slot.status])}
      </span>
      <div
        className="checkins-scores"
        role="group"
        aria-label={`${slot.label} ${slot.slot}`}
      >
        {[1, 2, 3, 4, 5].map((v) => (
          <button
            key={v}
            type="button"
            className={`checkins-score${slot.value === v ? " on" : ""}`}
            aria-pressed={slot.value === v}
            aria-label={`${slot.label} ${slot.slot} ${v}`}
            disabled={slot.status === "upcoming" || answer.isPending}
            onClick={() => pick(v)}
          >
            {v}
          </button>
        ))}
      </div>
    </li>
  );
}

function TrendCard({
  item,
  from,
  to,
}: {
  item: CheckinSeries;
  from: string;
  to: string;
}) {
  const t = useT();
  const pts = linePoints(item.points, from, to);
  return (
    <article className="xc-card checkins-trend-card" aria-label={item.label}>
      <div className="checkins-trend-title">
        <strong>{item.label}</strong>
        <span>
          {fmtScore(item.average)}{" "}
          <small className="xc-muted">{`${item.count} ${t("answers")}`}</small>
        </span>
      </div>
      {pts.length === 0 ? (
        <p className="xc-muted">{t("No answers in this range.")}</p>
      ) : (
        <svg
          viewBox={`0 0 ${CHART_W} ${CHART_H}`}
          className="checkins-chart"
          role="img"
          aria-label={item.label}
        >
          {[1, 3, 5].map((v) => (
            <line
              key={v}
              x1={8}
              x2={CHART_W - 8}
              y1={CHART_H - 8 - ((v - 1) / 4) * (CHART_H - 16)}
              y2={CHART_H - 8 - ((v - 1) / 4) * (CHART_H - 16)}
              className="checkins-grid"
            />
          ))}
          {pts.length > 1 && <path d={pathOf(pts)} className="checkins-line" />}
          {pts.map((p) => (
            <circle
              key={p.date}
              cx={p.x}
              cy={p.y}
              r={3}
              className="checkins-dot"
            >
              <title>{`${p.date}: ${fmtScore(p.average)}`}</title>
            </circle>
          ))}
        </svg>
      )}
    </article>
  );
}

function QuestionList({
  items,
  onEdit,
}: {
  items: CheckinQuestion[];
  onEdit: (q: CheckinQuestion) => void;
}) {
  const t = useT();
  const remove = useDeleteCheckinQuestion();
  const onDelete = async (q: CheckinQuestion) => {
    const ok = await confirmAction({
      title: `${t("Delete question")} ${q.label}？`,
      description: t("Its answers are deleted too."),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(q.id, {
      onSuccess: () => toast(t("Deleted")),
      onError: fail,
    });
  };
  return (
    <ul className="checkins-questions">
      {items.map((q) => (
        <li key={q.id}>
          <div className="checkins-slot-main">
            <strong>{q.label}</strong>
            <small className="xc-muted">
              {q.times.length ? q.times.join("、") : t("No time set")}
              {!q.enabled && ` · ${t("Question off")}`}
            </small>
          </div>
          <button
            className="icon-button"
            title={t("Edit question")}
            aria-label={`${t("Edit question")} ${q.label}`}
            onClick={() => onEdit(q)}
          >
            <Pencil size={14} />
          </button>
          <button
            className="icon-button"
            title={t("Delete question")}
            aria-label={`${t("Delete question")} ${q.label}`}
            onClick={() => onDelete(q)}
          >
            <Trash2 size={14} />
          </button>
        </li>
      ))}
    </ul>
  );
}
