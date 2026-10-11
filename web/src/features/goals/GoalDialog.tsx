import { useEffect, useState, type FormEvent } from "react";
import { Plus, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useGoalMetrics,
  useSaveGoal,
  type Goal,
  type GoalMetric,
  type GoalPeriod,
  type GoalSource,
} from "./api";
import {
  SOURCES,
  SOURCE_LABELS,
  SOURCE_UNITS,
  currentPeriod,
  fmtNum,
  parseNumber,
  quarterLabel,
  shiftPeriod,
} from "./format";

interface Row {
  key: number;
  id?: number;
  name: string;
  source: GoalSource;
  ref: string;
  target: string;
  unit: string;
  start: string;
}

let nextKey = 1;

function emptyRow(): Row {
  return {
    key: nextKey++,
    name: "",
    source: "manual",
    ref: "",
    target: "",
    unit: "",
    start: "0",
  };
}

interface Props {
  open: boolean;
  onClose: () => void;
  /** 传了就是修改 */
  goal?: Goal | null;
  /** 新建时默认的季度 */
  period: GoalPeriod;
}

/** 新建或修改一个目标，以及它的关键结果。 */
export default function GoalDialog({ open, onClose, goal, period }: Props) {
  const t = useT();
  const save = useSaveGoal();
  const metrics = useGoalMetrics(open);
  const [title, setTitle] = useState("");
  const [quarter, setQuarter] = useState<GoalPeriod>(period);
  const [note, setNote] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setTitle(goal?.title ?? "");
    setQuarter(goal ? { year: goal.year, quarter: goal.quarter } : period);
    setNote(goal?.note ?? "");
    setRows(
      goal
        ? goal.results.map((r) => ({
            key: nextKey++,
            id: r.id,
            name: r.name,
            source: r.source,
            ref: r.ref,
            target: fmtNum(r.target),
            unit: r.unit,
            start: fmtNum(r.startValue),
          }))
        : [],
    );
  }, [open, goal, period]);

  const available = new Map<string, GoalMetric>(
    (metrics.data?.items ?? []).map((m) => [m.source, m]),
  );
  // 已有的关键结果用的来源现在用不了时也留在列表里，免得保存时被换掉
  const optionsFor = (row: Row) =>
    SOURCES.filter(
      (s) => s === "manual" || available.has(s) || s === row.source,
    );

  const update = (key: number, patch: Partial<Row>) =>
    setRows((list) =>
      list.map((r) => (r.key === key ? { ...r, ...patch } : r)),
    );
  const changeSource = (row: Row, source: GoalSource) => {
    const first = available.get(source)?.options[0]?.value ?? "";
    update(row.key, {
      source,
      ref:
        available.get(source)?.refKind === "none" || source === "manual"
          ? ""
          : first,
      unit:
        row.unit === "" || row.unit === SOURCE_UNITS[row.source]
          ? SOURCE_UNITS[source]
          : row.unit,
    });
  };

  const years = [quarter.year - 1, quarter.year, quarter.year + 1].filter(
    (y, i, all) => all.indexOf(y) === i,
  );

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim()) return setError(t("Please enter a goal name"));
    const results = [];
    for (const r of rows) {
      const target = parseNumber(r.target);
      if (!r.name.trim()) return setError(t("Every key result needs a name"));
      if (target == null)
        return setError(`${r.name}: ${t("Enter a target value")}`);
      const start = r.source === "manual" ? parseNumber(r.start) : 0;
      if (start == null)
        return setError(`${r.name}: ${t("Enter a starting value")}`);
      results.push({
        ...(r.id != null ? { id: r.id } : {}),
        name: r.name.trim(),
        source: r.source,
        ref: r.ref,
        target,
        unit: r.unit.trim(),
        startValue: start,
      });
    }
    try {
      await save.mutateAsync({
        id: goal?.id,
        body: {
          title: title.trim(),
          year: quarter.year,
          quarter: quarter.quarter,
          note,
          results,
        },
      });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const hasGitHub = rows.some(
    (r) => r.source === "github_commits" || r.source === "github_merged_prs",
  );
  const thisQuarter = currentPeriod();

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={goal ? t("Edit goal") : t("New goal")}
      wide
    >
      <form onSubmit={submit} className="goals-form">
        <label className="xc-field">
          <span>{t("Goal name")}</span>
          <input
            className="xc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            maxLength={200}
            autoFocus
            placeholder="例如 把身体练起来"
          />
        </label>
        <div className="goals-form-row">
          <label className="xc-field">
            <span>{t("Goal year")}</span>
            <select
              className="xc-select"
              value={quarter.year}
              onChange={(e) =>
                setQuarter({ ...quarter, year: Number(e.target.value) })
              }
            >
              {years.map((y) => (
                <option key={y} value={y}>
                  {y}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field">
            <span>{t("Goal quarter")}</span>
            <select
              className="xc-select"
              value={quarter.quarter}
              onChange={(e) =>
                setQuarter({ ...quarter, quarter: Number(e.target.value) })
              }
            >
              {[1, 2, 3, 4].map((q) => (
                <option key={q} value={q}>
                  {`第 ${q} 季度`}
                </option>
              ))}
            </select>
          </label>
        </div>
        {!goal && (
          <div className="goals-hint-row">
            <button
              type="button"
              className="xc-btn small"
              onClick={() => setQuarter(thisQuarter)}
            >
              {quarterLabel(thisQuarter)}
            </button>
            <button
              type="button"
              className="xc-btn small"
              onClick={() => setQuarter(shiftPeriod(thisQuarter, 1))}
            >
              {quarterLabel(shiftPeriod(thisQuarter, 1))}
            </button>
          </div>
        )}
        <label className="xc-field">
          <span>{t("Goal note")}</span>
          <textarea
            className="xc-textarea"
            rows={2}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            maxLength={5000}
            placeholder="为什么要做这件事，做到什么样算成"
          />
        </label>

        <div className="goals-rows-head">
          <strong>{t("Key results")}</strong>
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setRows((l) => [...l, emptyRow()])}
          >
            <Plus size={14} /> {t("Add key result")}
          </button>
        </div>
        {rows.length === 0 && (
          <p className="goals-empty-line">
            {t(
              "A key result is a number you can check, such as 3 runs a week or 20 focus hours.",
            )}
          </p>
        )}
        {rows.map((r) => {
          const metric = available.get(r.source);
          return (
            <fieldset key={r.key} className="goals-row">
              <div className="goals-form-row">
                <label className="xc-field goals-grow">
                  <span>{t("Key result name")}</span>
                  <input
                    className="xc-input"
                    value={r.name}
                    onChange={(e) => update(r.key, { name: e.target.value })}
                    maxLength={100}
                    placeholder="例如 每周跑步三次"
                  />
                </label>
                <button
                  type="button"
                  className="xc-btn ghost small goals-row-remove"
                  aria-label={`${t("Remove key result")} ${r.name}`}
                  title={t("Remove key result")}
                  onClick={() =>
                    setRows((l) => l.filter((x) => x.key !== r.key))
                  }
                >
                  <X size={14} />
                </button>
              </div>
              <div className="goals-form-row">
                <label className="xc-field goals-grow">
                  <span>{t("Where the number comes from")}</span>
                  <select
                    className="xc-select"
                    value={r.source}
                    aria-label={`${r.name || t("Key result name")} ${t("Where the number comes from")}`}
                    onChange={(e) =>
                      changeSource(r, e.target.value as GoalSource)
                    }
                  >
                    {optionsFor(r).map((s) => (
                      <option key={s} value={s}>
                        {t(SOURCE_LABELS[s])}
                      </option>
                    ))}
                  </select>
                </label>
                {metric && metric.refKind !== "none" && (
                  <label className="xc-field goals-grow">
                    <span>
                      {metric.refKind === "habit"
                        ? t("Habit")
                        : metric.refKind === "skill"
                          ? t("Skill")
                          : t("Category")}
                    </span>
                    <select
                      className="xc-select"
                      value={r.ref}
                      onChange={(e) => update(r.key, { ref: e.target.value })}
                    >
                      {metric.options.length === 0 && (
                        <option value="">{t("Nothing to choose yet")}</option>
                      )}
                      {metric.options.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
              </div>
              <div className="goals-form-row">
                {r.source === "manual" && (
                  <label className="xc-field goals-narrow">
                    <span>{t("Starting value")}</span>
                    <input
                      className="xc-input"
                      inputMode="decimal"
                      value={r.start}
                      onChange={(e) => update(r.key, { start: e.target.value })}
                    />
                  </label>
                )}
                <label className="xc-field goals-narrow">
                  <span>{t("Target value")}</span>
                  <input
                    className="xc-input"
                    inputMode="decimal"
                    aria-label={`${r.name || t("Key result name")} ${t("Target value")}`}
                    value={r.target}
                    onChange={(e) => update(r.key, { target: e.target.value })}
                  />
                </label>
                <label className="xc-field goals-narrow">
                  <span>{t("Unit")}</span>
                  <input
                    className="xc-input"
                    value={r.unit}
                    onChange={(e) => update(r.key, { unit: e.target.value })}
                    maxLength={20}
                    placeholder="次、本、分钟"
                  />
                </label>
              </div>
            </fieldset>
          );
        })}
        {hasGitHub && (
          <p className="goals-hint">
            GitHub 只统计面板已经同步下来的最近提交和 PR，次数可能比实际少。
          </p>
        )}
        <p className="goals-hint">
          自动统计的数字从这个季度的第一天开始累计。手动填写的可以设起点，比如体重从
          80 减到 75。
        </p>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
