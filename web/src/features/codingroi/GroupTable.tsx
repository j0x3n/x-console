import { useMemo, useState } from "react";
import { useT } from "../../contexts/LanguageContext";
import type { RoiGroup, RoiGroupBy } from "./api";
import { SortTh, Value } from "./cells";
import {
  breakdown,
  groupCell,
  minutesText,
  moneyText,
  percentText,
  secondsText,
  sortRows,
  tokensText,
  type Sort,
} from "./format";

const FIRST_COLUMN: Record<RoiGroupBy, string> = {
  repo: "Repository",
  tool: "Coding tool",
  agent: "Agents",
};

/** 分组汇总表。每列可排序，合计固定在最下面。 */
export default function GroupTable({
  groups,
  totals,
  groupBy,
}: {
  groups: RoiGroup[];
  totals: RoiGroup;
  groupBy: RoiGroupBy;
}) {
  const t = useT();
  const [sort, setSort] = useState<Sort>({ key: "tasks", desc: true });
  const rows = useMemo(() => sortRows(groups, sort, groupCell), [groups, sort]);
  const th = (label: string, col: string, title?: string) => (
    <SortTh
      label={t(label)}
      col={col}
      sort={sort}
      onSort={setSort}
      title={title ? t(title) : undefined}
      className={col === "label" ? "" : "roi-num"}
    />
  );
  return (
    <section
      className="xc-card roi-card"
      aria-label={t("Groups of this period")}
    >
      <div className="xc-card-head">
        <h3>{t("Groups of this period")}</h3>
      </div>
      <div className="xc-table-wrap">
        <table className="xc-table roi-table roi-groups">
          <thead>
            <tr>
              {th(FIRST_COLUMN[groupBy], "label")}
              {th("Tasks", "tasks")}
              {th("Merged", "merged")}
              {th("Reverted", "reverted")}
              {th("Cost", "cost")}
              {th("AI run time", "run")}
              {th("Review time (estimate)", "review")}
              {th("Rework rounds", "rounds")}
              {th("Cost per merge", "costPerMerge")}
              {th("Run time per merge", "runPerMerge")}
              {th("Review time per merge", "reviewPerMerge")}
            </tr>
          </thead>
          <tbody>
            {rows.map((g) => (
              <GroupRow
                key={g.key || "none"}
                g={g}
                label={
                  groupBy === "agent" && !g.key ? t("Manual tasks") : g.label
                }
              />
            ))}
          </tbody>
          <tfoot>
            <GroupRow g={totals} label={t("Sum of all groups")} total />
          </tfoot>
        </table>
      </div>
    </section>
  );
}

function GroupRow({
  g,
  label,
  total,
}: {
  g: RoiGroup;
  label: string;
  total?: boolean;
}) {
  const t = useT();
  const sub = breakdown(t, g);
  // 有任务没有记录时，合计只含有记录的，数字是下限。
  const lowCost = g.costMissing > 0;
  const lowRun = g.runMissing > 0;
  const lowReview = g.reviewMissing > 0;
  return (
    <tr className={total ? "roi-total" : undefined}>
      <td className="roi-name">
        <strong title={label}>{label}</strong>
        {sub && <small>{sub}</small>}
      </td>
      <td className="roi-num">{g.tasks}</td>
      <td className="roi-num">
        {g.merged}
        {g.mergeRate != null && (
          <small className="roi-sub">
            {t("Merge rate")} {percentText(g.mergeRate)}
          </small>
        )}
      </td>
      <td className="roi-num">{g.reverted}</td>
      <td className="roi-num">
        <Value text={moneyText(g.cost)} atLeast={lowCost && g.cost != null} />
        {g.inputTokens != null && (
          <small className="roi-sub">
            {tokensText(g.inputTokens + (g.outputTokens ?? 0))} {t("Tokens")}
          </small>
        )}
      </td>
      <td className="roi-num">
        <Value
          text={secondsText(t, g.runSeconds)}
          atLeast={lowRun && g.runSeconds != null}
        />
      </td>
      <td className="roi-num">
        <Value
          text={minutesText(t, g.reviewMinutes)}
          atLeast={lowReview && g.reviewMinutes != null}
        />
      </td>
      <td className="roi-num">{g.rounds}</td>
      <td className="roi-num">
        <Value text={moneyText(g.costPerMerge)} atLeast={lowCost} />
      </td>
      <td className="roi-num">
        <Value text={secondsText(t, g.runSecondsPerMerge)} atLeast={lowRun} />
      </td>
      <td className="roi-num">
        <Value
          text={minutesText(t, g.reviewMinutesPerMerge)}
          atLeast={lowReview}
        />
      </td>
    </tr>
  );
}
