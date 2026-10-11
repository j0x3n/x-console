import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import type { RoiTask } from "./api";
import { NotApplicable, SortTh, Value } from "./cells";
import {
  OUTCOME_LABELS,
  OUTCOME_TONES,
  minutesText,
  moneyText,
  ratioText,
  secondsText,
  sortRows,
  taskCell,
  tokensText,
  type Sort,
} from "./format";

/** 任务明细。每列可排序，标题点进任务页。 */
export default function TaskTable({ tasks }: { tasks: RoiTask[] }) {
  const t = useT();
  const [sort, setSort] = useState<Sort>({ key: "created", desc: true });
  const rows = useMemo(() => sortRows(tasks, sort, taskCell), [tasks, sort]);
  const th = (label: string, col: string, title?: string) => (
    <SortTh
      label={t(label)}
      col={col}
      sort={sort}
      onSort={setSort}
      title={title ? t(title) : undefined}
      className={col === "title" || col === "outcome" ? "" : "roi-num"}
    />
  );
  return (
    <section
      className="xc-card roi-card"
      aria-label={t("Tasks of this period")}
    >
      <div className="xc-card-head">
        <h3>{t("Tasks of this period")}</h3>
      </div>
      <div className="xc-table-wrap">
        <table className="xc-table roi-table roi-tasks">
          <thead>
            <tr>
              {th("Task title", "title")}
              {th("Result", "outcome")}
              {th("Cost", "cost")}
              {th("Tokens", "tokens")}
              {th("AI run time", "run")}
              {th("Review time (estimate)", "review")}
              {th("Rework rounds", "rounds")}
              {th("Review per run", "reviewPerRun")}
            </tr>
          </thead>
          <tbody>
            {rows.map((task) => (
              <TaskRow key={task.id} task={task} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function TaskRow({ task }: { task: RoiTask }) {
  const t = useT();
  const language = useLanguage();
  // 还没开始的任务没有费用和运行时间，不是“没记录”。
  const ran = !!task.startedAt;
  const tool = task.executor === "codex" ? "Codex" : "Claude Code";
  const meta = [task.repoName, tool, task.agentName ?? t("Manual tasks")];
  const tokens =
    task.inputTokens == null
      ? null
      : tokensText(task.inputTokens + (task.outputTokens ?? 0));
  return (
    <tr>
      <td className="roi-name">
        <Link to={`/coding/${task.id}`} title={t("Open the task")}>
          <strong>{task.title || `#${task.id}`}</strong>
        </Link>
        <small>
          {meta.join(" · ")} · {relativeTime(task.createdAt, language)}
        </small>
      </td>
      <td>
        <span
          className={`xc-badge ${OUTCOME_TONES[task.outcome]}`.trim()}
          title={task.status}
        >
          {t(OUTCOME_LABELS[task.outcome])}
        </span>
      </td>
      <td className="roi-num">
        {ran ? (
          <Value
            text={moneyText(task.cost)}
            note={
              task.costPartial && (
                <small
                  className="roi-sub"
                  title={t("Some calls have no cost record")}
                >
                  {t("Partial cost")}
                </small>
              )
            }
          />
        ) : (
          <NotApplicable />
        )}
      </td>
      <td className="roi-num">
        {ran ? <Value text={tokens} /> : <NotApplicable />}
      </td>
      <td className="roi-num">
        {ran ? (
          <Value text={secondsText(t, task.runSeconds)} />
        ) : (
          <NotApplicable />
        )}
      </td>
      <td className="roi-num">
        <ReviewCell task={task} />
      </td>
      <td className="roi-num">
        {task.rounds == null ? <NotApplicable /> : task.rounds}
      </td>
      <td className="roi-num">
        {task.reviewPerRun == null ? (
          <NotApplicable />
        ) : (
          ratioText(task.reviewPerRun)
        )}
      </td>
    </tr>
  );
}

function ReviewCell({ task }: { task: RoiTask }) {
  const t = useT();
  if (task.reviewState === "none") return <NotApplicable />;
  if (task.reviewState === "no_data") return <Value text={null} />;
  return (
    <Value
      text={minutesText(t, task.reviewMinutes)}
      note={
        task.reviewState === "ongoing" && (
          <small
            className="roi-sub"
            title={t("The part still being reviewed is counted up to now")}
          >
            {t("In progress")}
          </small>
        )
      }
    />
  );
}
