import { Link } from "react-router";
import { AlarmClock, Check, Code2, SquareKanban } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatTime, relativeTime } from "../../../lib/time";
import { useTasks } from "../../coding/api";
import { useAgentDecisions } from "../../aiagents/api";
import { DecisionRow } from "../../aiagents/DecisionRows";
import { useMyIssues, type Issue } from "../../projects/api";
import { PRIORITY_LABELS } from "../../projects/logic";
import {
  useCompleteReminder,
  useReminderCounts,
  useReminders,
} from "../../reminders/api";
import { daysLate, dueToday, overdue } from "../today";
import { Empty, MoreLink, Pending, QueryState } from "./shared";

const LIMIT = 8;

function issuePath(issue: Issue) {
  return `/projects/${issue.projectKey}/${issue.number}`;
}

function IssueRow({ issue, late }: { issue: Issue; late?: number }) {
  const t = useT();
  return (
    <Link className="today-row" to={issuePath(issue)}>
      <span className="today-row-icon">
        <SquareKanban size={15} />
      </span>
      <span className="today-row-main">
        <strong>{issue.title}</strong>
        <small>
          {issue.key}
          {issue.priority > 0 && issue.priority <= 2 && (
            <> · {t(PRIORITY_LABELS[issue.priority])}</>
          )}
        </small>
      </span>
      {late ? (
        <span className="xc-badge danger">
          {t("Overdue by")} {late} {t("days")}
        </span>
      ) : (
        <span className="xc-badge">{t("Due today")}</span>
      )}
    </Link>
  );
}

/** 今日待办：今天截止的 Issue 和今天还没完成的提醒。 */
export function TodosCard() {
  const t = useT();
  const language = useLanguage();
  const issues = useMyIssues();
  const reminders = useReminders("today");
  const complete = useCompleteReminder();
  const counts = useReminderCounts();
  const now = new Date();
  const todayIssues = dueToday(issues.data ?? [], now);
  const todayReminders = (reminders.data ?? []).filter(
    (r) => r.status !== "done" && r.status !== "ended",
  );
  // 其他模块今天到期的事项（订阅续费、证书到期这类），点了去来源页面
  const external = counts.externalToday;
  const total = todayIssues.length + todayReminders.length + external.length;
  const loading = issues.isPending || reminders.isPending;

  return (
    <>
      {loading ? (
        <Pending />
      ) : (
        <>
          {issues.isError && <QueryState query={issues} />}
          {reminders.isError && <QueryState query={reminders} />}
          {total === 0 && !issues.isError && !reminders.isError ? (
            <Empty>{t("Nothing due today")}</Empty>
          ) : (
            total > 0 && (
              <div className="xc-list">
                {todayReminders.slice(0, LIMIT).map((r) => (
                  <div className="today-row" key={`r${r.id}`}>
                    <span className="today-row-icon">
                      <AlarmClock size={15} />
                    </span>
                    <Link className="today-row-main" to="/reminders">
                      <strong>{r.title}</strong>
                      <small>
                        {formatTime(r.dueAt ?? r.dtstart, language)}
                        {r.status === "pending" && <> · {t("Time is up")}</>}
                      </small>
                    </Link>
                    <button
                      className="xc-btn small ghost"
                      title={t("Mark done")}
                      aria-label={t("Mark done")}
                      disabled={complete.isPending}
                      onClick={() =>
                        complete.mutate(r.id, {
                          onSuccess: () => toast(t("Marked done")),
                          onError: (e) =>
                            toast({ message: errorMessage(e), tone: "error" }),
                        })
                      }
                    >
                      <Check size={14} />
                    </button>
                  </div>
                ))}
                {external.slice(0, LIMIT).map((x) => (
                  <Link className="today-row" key={`x${x.id}`} to={x.link}>
                    <span className="today-row-icon">
                      <AlarmClock size={15} />
                    </span>
                    <span className="today-row-main">
                      <strong>{x.title}</strong>
                      <small>
                        {x.sourceLabel} · {formatTime(x.at, language)}
                      </small>
                    </span>
                  </Link>
                ))}
                {todayIssues.slice(0, LIMIT).map((i) => (
                  <IssueRow key={`i${i.id}`} issue={i} />
                ))}
              </div>
            )
          )}
        </>
      )}
    </>
  );
}

export function useTodoCount() {
  const issues = useMyIssues();
  // 和提醒页“今天”标签的数一致，包括其他模块今天到期的事项
  const reminders = useReminderCounts();
  const now = new Date();
  return {
    issues: issues.data ? dueToday(issues.data, now).length : undefined,
    overdue: issues.data ? overdue(issues.data, now).length : undefined,
    reminders: reminders.loading ? undefined : reminders.today,
  };
}

/** 待你决定：等你确认的 Agent 任务和已经逾期的 Issue。 */
export function DecisionsCard() {
  const t = useT();
  const language = useLanguage();
  const tasks = useTasks(["review"]);
  const issues = useMyIssues();
  // B87：Agent 的权限请求和问题，接口没上线时当成没有
  const decisions = useAgentDecisions();
  const asks = Array.isArray(decisions.data) ? decisions.data : [];
  const now = new Date();
  const late = overdue(issues.data ?? [], now);
  const review = tasks.data ?? [];
  const total = asks.length + review.length + late.length;

  if (tasks.isPending || issues.isPending) return <Pending />;
  return (
    <>
      {tasks.isError && <QueryState query={tasks} />}
      {issues.isError && <QueryState query={issues} />}
      {total === 0 && !tasks.isError && !issues.isError && (
        <Empty>{t("Nothing needs your call")}</Empty>
      )}
      {total > 0 && (
        <div className="xc-list">
          {asks.slice(0, LIMIT).map((d) => (
            <DecisionRow key={`d${d.id}`} decision={d} />
          ))}
          {review.slice(0, LIMIT).map((task) => (
            <Link
              className="today-row"
              key={`t${task.id}`}
              to={`/coding/${task.id}`}
            >
              <span className="today-row-icon accent">
                <Code2 size={15} />
              </span>
              <span className="today-row-main">
                <strong>{task.title}</strong>
                <small>
                  {task.repoName} · {task.changedFiles.length}{" "}
                  {t("files changed")} ·{" "}
                  {relativeTime(task.finishedAt ?? task.updatedAt, language)}
                </small>
              </span>
              <span className="xc-badge accent">{t("Needs review")}</span>
            </Link>
          ))}
          {late.slice(0, LIMIT).map((i) => (
            <IssueRow
              key={`i${i.id}`}
              issue={i}
              late={daysLate(i.dueDate ?? "", now)}
            />
          ))}
        </div>
      )}
      {late.length > LIMIT && <MoreLink to="/projects" />}
    </>
  );
}
