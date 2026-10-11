import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { toast } from "../../hooks/useToast";
import { errorMessage } from "../../api/client";
import { useDayPlan, useSetItemDone } from "./api";
import { doneCount, rangeText } from "./format";
import "./i18n";
import "./plan.css";

/** 今日页是否显示“今日计划”：今天有计划才显示（B135）。 */
export function useHasTodayPlan() {
  const q = useDayPlan();
  return (q.data?.plan?.items.length ?? 0) > 0;
}

/** 今日页的“今日计划”卡片：事项、时间和勾选。 */
export default function TodayPlanCard() {
  const t = useT();
  const q = useDayPlan();
  const done = useSetItemDone();
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const plan = q.data.plan;
  if (!plan) return null;
  return (
    <div className="plan-today">
      {plan.status === "draft" && (
        <small className="xc-muted">{t("Draft, not confirmed yet")}</small>
      )}
      <ul>
        {plan.items.map((it) => (
          <li key={it.id}>
            <label className="plan-today-row">
              <input
                type="checkbox"
                checked={it.done}
                aria-label={`${t("Plan item done")} ${it.title}`}
                disabled={done.isPending}
                onChange={(e) =>
                  done.mutate(
                    { id: it.id, done: e.target.checked },
                    {
                      onError: (err) =>
                        toast({ message: errorMessage(err), tone: "error" }),
                    },
                  )
                }
              />
              <span className={it.done ? "plan-done" : undefined}>
                {it.title}
              </span>
              <small className="xc-muted">{rangeText(it.start, it.end)}</small>
            </label>
          </li>
        ))}
      </ul>
      <small className="xc-muted">
        {doneCount(plan.items)} / {plan.items.length} {t("done")} ·{" "}
        <Link to="/plan">{t("Open the plan")}</Link>
      </small>
    </div>
  );
}
