import { useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router";
import {
  Bell,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Target,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { Toolbar } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useGoalReview,
  useGoalReviews,
  useGoalSettings,
  usePutGoalSettings,
  useSaveReview,
  type GoalWeekGoal,
} from "./api";
import { ProgressBar } from "./GoalCard";
import GoalTabs from "./GoalTabs";
import {
  SOURCE_LABELS,
  STATE_LABELS,
  fmtChange,
  fmtPercent,
  shiftWeek,
  stateTone,
  valueText,
  weekLabel,
} from "./format";
import "./i18n";
import "./goals.css";

/** 每周复盘（B128）：每个目标这周的变化，三个固定问题，往期记录。 */
export default function ReviewPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const week = params.get("week") ?? "current";
  const review = useGoalReview(week);
  const history = useGoalReviews();
  const save = useSaveReview();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [wentWell, setWentWell] = useState("");
  const [fellShort, setFellShort] = useState("");
  const [nextWeek, setNextWeek] = useState("");

  const data = review.data;
  // 换了一周或保存过之后，输入框回到服务端存的内容。进度刷新不会动正在写的字。
  useEffect(() => {
    if (!data) return;
    setWentWell(data.wentWell);
    setFellShort(data.fellShort);
    setNextWeek(data.nextWeek);
  }, [data?.weekStart, data?.updatedAt]); // eslint-disable-line react-hooks/exhaustive-deps

  const goWeek = (target: string | null) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (target == null) next.delete("week");
        else next.set("week", target);
        return next;
      },
      { replace: true },
    );

  const dirty =
    data != null &&
    (wentWell !== data.wentWell ||
      fellShort !== data.fellShort ||
      nextWeek !== data.nextWeek);
  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    if (!data) return;
    save.mutate(
      { week: data.weekStart, body: { wentWell, fellShort, nextWeek } },
      {
        onSuccess: () => toast(t("Review saved")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  let body;
  if (review.isPending) body = <Loading />;
  else if (review.isError && isNotLive(review.error))
    body = (
      <NotLive name={t("Weekly review")} icon={<ClipboardCheck size={28} />} />
    );
  else if (review.isError)
    body = <ErrorState error={review.error} onRetry={() => review.refetch()} />;
  else if (data)
    body = (
      <div className="goals-review">
        <form className="goals-review-main" onSubmit={submit}>
          <section className="goals-week-nav" aria-label={t("Week")}>
            <button
              type="button"
              className="xc-btn ghost small"
              aria-label={t("Previous week")}
              title={t("Previous week")}
              onClick={() => goWeek(shiftWeek(data.weekStart, -1))}
            >
              <ChevronLeft size={15} />
            </button>
            <strong>{weekLabel(data.weekStart, data.weekEnd)}</strong>
            {data.current && (
              <span className="xc-badge accent">{t("This week")}</span>
            )}
            <button
              type="button"
              className="xc-btn ghost small"
              aria-label={t("Next week")}
              title={t("Next week")}
              disabled={data.current}
              onClick={() => goWeek(shiftWeek(data.weekStart, 1))}
            >
              <ChevronRight size={15} />
            </button>
            {!data.current && (
              <button
                type="button"
                className="xc-btn small"
                onClick={() => goWeek(null)}
              >
                {t("This week")}
              </button>
            )}
          </section>

          {data.goals.length === 0 ? (
            <EmptyState
              title={t("No active goals this week")}
              icon={<Target size={28} />}
            >
              <span>这周没有进行中的目标。你仍然可以写下面三个问题。</span>
              <Link className="xc-btn small" to="/goals?new=1">
                {t("New goal")}
              </Link>
            </EmptyState>
          ) : (
            data.goals.map((g) => <WeekGoal key={g.goalId} goal={g} />)
          )}

          {data.lastPlan.trim() && (
            <section className="xc-card goals-lastplan">
              <div className="xc-card-head">
                <strong>{t("What you planned last week")}</strong>
              </div>
              <Markdown source={data.lastPlan} />
            </section>
          )}

          <section className="xc-card goals-questions">
            <div className="xc-field">
              <span>{t("What went well this week?")}</span>
              <MarkdownEditor
                label={t("What went well this week?")}
                value={wentWell}
                onChange={setWentWell}
                minRows={3}
                onSubmit={() => submit()}
              />
            </div>
            <div className="xc-field">
              <span>{t("What did not get done?")}</span>
              <MarkdownEditor
                label={t("What did not get done?")}
                value={fellShort}
                onChange={setFellShort}
                minRows={3}
                onSubmit={() => submit()}
              />
            </div>
            <div className="xc-field">
              <span>{t("When will you do what next week?")}</span>
              <small>写成“周二晚上 8 点，读书一小时”这样，越具体越好。</small>
              <MarkdownEditor
                label={t("When will you do what next week?")}
                value={nextWeek}
                onChange={setNextWeek}
                minRows={3}
                onSubmit={() => submit()}
              />
            </div>
            <div className="goals-save-row">
              <small>
                {data.saved && data.updatedAt
                  ? `${t("Last saved")} ${new Date(data.updatedAt).toLocaleString()}`
                  : t("Not saved yet")}
              </small>
              <button
                type="submit"
                className="xc-btn primary"
                disabled={save.isPending || (!dirty && data.saved)}
              >
                {t("Save review")}
              </button>
            </div>
          </section>
          <p className="goals-footnote">
            把“什么时候做什么”写具体，通常比只写“多做点”更容易做到。效果有多大还有争议，这里只是帮你留个记录。
          </p>
        </form>

        <aside className="goals-history" aria-label={t("Past reviews")}>
          <h2>{t("Past reviews")}</h2>
          {history.data?.items.length ? (
            <ul>
              {history.data.items.map((h) => (
                <li key={h.weekStart}>
                  <button
                    type="button"
                    className={h.weekStart === data.weekStart ? "on" : ""}
                    onClick={() => goWeek(h.weekStart)}
                  >
                    <span>{weekLabel(h.weekStart, h.weekEnd)}</span>
                    <small>
                      {h.progress != null && `${fmtPercent(h.progress)} · `}
                      {h.answered ? t("Review written") : t("Review empty")}
                    </small>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="goals-empty-line">
              {t("Saved reviews show up here.")}
            </p>
          )}
        </aside>
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Weekly review")}
        subtitle={data ? weekLabel(data.weekStart, data.weekEnd) : undefined}
        aside={
          <button
            className="xc-btn"
            title={t("Reminder settings")}
            onClick={() => setSettingsOpen(true)}
          >
            <Bell size={15} /> {t("Reminder settings")}
          </button>
        }
      />
      <Toolbar start={<GoalTabs active="review" />} />
      {body}
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

function WeekGoal({ goal: g }: { goal: GoalWeekGoal }) {
  const t = useT();
  return (
    <section className="xc-card goals-week-goal">
      <header className="goals-card-head">
        <div className="goals-card-title">
          <strong>{g.title}</strong>
          <span className="goals-badges">
            <span className={`xc-badge ${stateTone(g.state)}`}>
              {t(STATE_LABELS[g.state])}
            </span>
          </span>
        </div>
        <span className="goals-change" title={t("Change this week")}>
          {fmtChange(g.change)}
          {g.change != null ? " %" : ""}
        </span>
      </header>
      <ProgressBar
        progress={g.progress}
        expected={g.expected}
        label={`${g.title} ${t("Goal progress")}`}
      />
      <small className="goals-pace">
        {g.progress == null
          ? t("No key result can be measured yet")
          : `${t("Goal progress")} ${fmtPercent(g.progress)} · ${t("Expected by now")} ${fmtPercent(g.expected)}${
              g.progress + 10 < g.expected
                ? ` · ${t("Behind by")} ${fmtPercent(g.expected - g.progress)}`
                : ""
            }`}
      </small>
      <ul className="goals-results">
        {g.results.map((r) => (
          <li key={r.id} className="goals-result">
            <div className="goals-result-main">
              <span className="goals-result-name">
                {r.name}
                {r.source !== "manual" && (
                  <small>{t(SOURCE_LABELS[r.source])}</small>
                )}
              </span>
              <span className="goals-result-value">
                {r.unavailable ? (
                  <small className="goals-unavailable">{t("No data")}</small>
                ) : (
                  <>
                    <span>{valueText(r.value, r.target, r.unit)}</span>
                    <span className="goals-delta">
                      {t("This week")} {fmtChange(r.change)}
                      {r.unit && r.change != null ? ` ${r.unit}` : ""}
                    </span>
                  </>
                )}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function SettingsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useGoalSettings();
  const put = usePutGoalSettings();
  const [on, setOn] = useState(true);
  const [time, setTime] = useState("20:00");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open || !settings.data) return;
    setOn(settings.data.reviewReminder);
    setTime(settings.data.reviewTime);
    setError("");
  }, [open, settings.data]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await put.mutateAsync({ reviewReminder: on, reviewTime: time });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("Reminder settings")}>
      <form onSubmit={submit}>
        <div className="goals-setting-row">
          <span>{t("Remind me to review on Sunday evening")}</span>
          <Switch
            checked={on}
            onChange={setOn}
            label={t("Remind me to review on Sunday evening")}
          />
        </div>
        <label className="xc-field">
          <span>{t("Reminder time")}</span>
          <input
            className="xc-input"
            type="time"
            value={time}
            disabled={!on}
            onChange={(e) => setTime(e.target.value)}
          />
          <small>
            用服务器的时区。到点发一条通知，点开就是这个页面。这周已经写了复盘或者没有进行中的目标，就不发。
          </small>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={put.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
