import { useEffect, useMemo, useState } from "react";
import { CalendarClock, Plus, Settings, Sparkles, Trash2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  toInputs,
  useConfirmPlan,
  useDayPlan,
  useDayPlanSettings,
  useGeneratePlan,
  useSavePlanItems,
  useSetItemDone,
  type DayPlanItemInput,
} from "./api";
import { checkItems, dateOffset, sameItems } from "./format";
import PlanSettingsDialog from "./PlanSettingsDialog";
import "./i18n";
import "./plan.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 今日计划（B135）：空档、AI 排的几件事、确认、晚上勾掉做完的。 */
export default function PlanPage() {
  const t = useT();
  const [day, setDay] = useState<"today" | "tomorrow">("today");
  const date = dateOffset(day === "today" ? 0 : 1);
  const q = useDayPlan(date);
  const settings = useDayPlanSettings();
  const generate = useGeneratePlan();
  const save = useSavePlanItems();
  const confirm = useConfirmPlan();
  const done = useSetItemDone();
  const [items, setItems] = useState<DayPlanItemInput[]>([]);
  const [error, setError] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);

  const plan = q.data?.plan;
  useEffect(() => {
    setItems(plan ? toInputs(plan.items) : []);
    setError("");
  }, [plan]);

  const dirty = useMemo(
    () => (plan ? !sameItems(plan.items, items) : items.length > 0),
    [plan, items],
  );

  if (q.isPending) return <Loading />;
  if (q.isError && isNotLive(q.error))
    return <NotLive name={t("Day plan")} icon={<CalendarClock size={28} />} />;
  if (q.isError)
    return <ErrorState error={q.error} onRetry={() => q.refetch()} />;

  const patch = (i: number, p: Partial<DayPlanItemInput>) =>
    setItems((list) => list.map((it, j) => (j === i ? { ...it, ...p } : it)));
  const confirmed = plan?.status === "confirmed";

  const runGenerate = () =>
    generate.mutate({ date, replace: confirmed }, { onError: fail });
  const persist = (then?: () => void) => {
    const problem = checkItems(items);
    if (problem) return setError(problem);
    setError("");
    save.mutate(
      { date, items },
      { onSuccess: then, onError: (e) => setError(errorMessage(e)) },
    );
  };
  const doConfirm = () => {
    if (dirty) return persist(() => confirm.mutate(date, { onError: fail }));
    confirm.mutate(date, { onError: fail });
  };

  return (
    <div className="xc-page plan-page">
      <PageHeading
        title={t("Day plan")}
        subtitle={
          plan
            ? confirmed
              ? t("Confirmed")
              : t("Draft, not confirmed yet")
            : undefined
        }
        aside={
          <>
            <Segmented<"today" | "tomorrow">
              label={t("Plan day")}
              value={day}
              onChange={setDay}
              options={[
                { value: "today", label: t("Today") },
                { value: "tomorrow", label: t("Tomorrow") },
              ]}
            />
            <button
              className="xc-btn small"
              onClick={() => setSettingsOpen(true)}
            >
              <Settings size={14} /> {t("Plan settings")}
            </button>
          </>
        }
      />
      <section className="xc-card plan-slots" aria-label={t("Free time")}>
        <strong>{t("Free time")}</strong>
        {q.data.slots.length === 0 ? (
          <span className="xc-muted">
            {t("No free stretch of 30 minutes or more.")}
          </span>
        ) : (
          <ul>
            {q.data.slots.map((s) => (
              <li key={s.start} className="plan-slot">
                {s.start}–{s.end}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="xc-card" aria-label={t("The plan")}>
        <div className="xc-card-head">
          <h2>{t("The plan")}</h2>
          {plan && (
            <span className="xc-muted plan-source">
              {t(SOURCE_LABELS[plan.source])}
            </span>
          )}
        </div>
        {!plan && items.length === 0 ? (
          <p className="xc-muted">
            {t("No plan yet. Let the planner pick the most important things.")}
          </p>
        ) : (
          <ol className="plan-items">
            {items.map((it, i) => (
              <li key={i} className="plan-item">
                <input
                  type="checkbox"
                  className="plan-check"
                  aria-label={`${t("Plan item done")} ${it.title}`}
                  checked={it.done ?? false}
                  disabled={!plan || dirty || done.isPending}
                  onChange={(e) => {
                    const id = plan?.items[i]?.id;
                    if (id != null)
                      done.mutate(
                        { id, done: e.target.checked },
                        { onError: fail },
                      );
                  }}
                />
                <div className="plan-item-main">
                  <input
                    className="xc-input"
                    value={it.title}
                    aria-label={t("Item title")}
                    maxLength={200}
                    onChange={(e) => patch(i, { title: e.target.value })}
                  />
                  {plan?.items[i]?.reason && (
                    <small className="xc-muted">{plan.items[i].reason}</small>
                  )}
                </div>
                <div className="plan-times">
                  <input
                    className="xc-input"
                    type="time"
                    aria-label={`${t("Plan start")} ${it.title}`}
                    value={it.start ?? ""}
                    onChange={(e) => patch(i, { start: e.target.value })}
                  />
                  <span>–</span>
                  <input
                    className="xc-input"
                    type="time"
                    aria-label={`${t("Plan end")} ${it.title}`}
                    value={it.end ?? ""}
                    onChange={(e) => patch(i, { end: e.target.value })}
                  />
                </div>
                <button
                  className="icon-button"
                  title={t("Remove item")}
                  aria-label={`${t("Remove item")} ${it.title}`}
                  onClick={() =>
                    setItems((list) => list.filter((_, j) => j !== i))
                  }
                >
                  <Trash2 size={14} />
                </button>
              </li>
            ))}
          </ol>
        )}
        {error && (
          <p className="xc-error" role="alert">
            {error}
          </p>
        )}
        <div className="plan-actions">
          <button
            className="xc-btn small"
            disabled={items.length >= 8}
            onClick={() =>
              setItems((list) => [...list, { title: "", kind: "manual" }])
            }
          >
            <Plus size={14} /> {t("Plan add item")}
          </button>
          <button
            className="xc-btn small"
            disabled={generate.isPending}
            onClick={runGenerate}
          >
            <Sparkles size={14} />{" "}
            {plan
              ? confirmed
                ? t("Plan again")
                : t("Plan again")
              : t("Plan the day")}
          </button>
          <button
            className="xc-btn small"
            disabled={!dirty || save.isPending}
            onClick={() => persist()}
          >
            {t("Save changes")}
          </button>
          {!confirmed && (
            <button
              className="xc-btn small primary"
              disabled={items.length === 0 || confirm.isPending}
              onClick={doConfirm}
            >
              {t("Confirm the plan")}
            </button>
          )}
        </div>
        <p className="xc-muted plan-hint">
          {t("The calendar is only read. The plan is not written back to it.")}
        </p>
      </section>
      <PlanSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings.data}
      />
    </div>
  );
}

const SOURCE_LABELS = {
  ai: "Plan by AI",
  rule: "Plan by rule",
  manual: "Plan edited by you",
} as const;
