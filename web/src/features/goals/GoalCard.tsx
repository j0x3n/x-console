import { useState, type FormEvent } from "react";
import { Check, Pencil, RotateCcw, Trash2, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDeleteGoal,
  useSetGoalStatus,
  useSetResultValue,
  type Goal,
  type GoalResult,
} from "./api";
import {
  SOURCE_LABELS,
  STATE_LABELS,
  barWidths,
  fmtNum,
  fmtPercent,
  parseNumber,
  stateTone,
  valueText,
} from "./format";

const UNAVAILABLE: Record<string, string> = {
  hidden: "Source is not available",
  missing: "Source is not set up",
  error: "Could not read the source",
};

/** 进度条。黑块是已到的，竖线是按时间应该到的位置。 */
export function ProgressBar({
  progress,
  expected,
  label,
}: {
  progress: number | null | undefined;
  expected?: number;
  label: string;
}) {
  const { fill, mark } = barWidths(progress, expected ?? 0);
  return (
    <div
      className="goals-bar"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={progress == null ? undefined : Math.round(progress)}
    >
      <i style={{ width: `${fill}%` }} />
      {expected != null && <b style={{ left: `${mark}%` }} />}
    </div>
  );
}

interface Props {
  goal: Goal;
  onEdit: () => void;
}

/** 一个目标：进度条、按时间应该到哪、关键结果。 */
export default function GoalCard({ goal: g, onEdit }: Props) {
  const t = useT();
  const setStatus = useSetGoalStatus();
  const remove = useDeleteGoal();
  const dim = g.status !== "active";

  const change = (status: "active" | "done" | "dropped", done: string) =>
    setStatus.mutate(
      { id: g.id, status },
      {
        onSuccess: () => toast(done),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete this goal?")} ${g.title}`,
      description: t(
        "The goal and its key results are deleted. Past reviews keep what they saved.",
      ),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(g.id, {
      onSuccess: () => toast(t("Deleted")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };

  const items = [
    {
      key: "edit",
      label: t("Edit"),
      icon: <Pencil size={14} />,
      onSelect: onEdit,
    },
    g.status === "active"
      ? {
          key: "done",
          label: t("Mark as reached"),
          icon: <Check size={14} />,
          onSelect: () => change("done", t("Marked as reached")),
        }
      : {
          key: "resume",
          label: t("Work on it again"),
          icon: <RotateCcw size={14} />,
          onSelect: () => change("active", t("Saved")),
        },
    ...(g.status === "active"
      ? [
          {
            key: "drop",
            label: t("Give it up"),
            icon: <X size={14} />,
            onSelect: () => change("dropped", t("Saved")),
          },
        ]
      : []),
    {
      key: "delete",
      label: t("Delete"),
      icon: <Trash2 size={14} />,
      danger: true,
      onSelect: onDelete,
    },
  ];

  return (
    <article className={`xc-card goals-card${dim ? " dim" : ""}`}>
      <header className="goals-card-head">
        <div className="goals-card-title">
          <strong>{g.title}</strong>
          <span className="goals-badges">
            {g.status === "done" && (
              <span className="xc-badge ok">{t("Marked as reached")}</span>
            )}
            {g.status === "dropped" && (
              <span className="xc-badge">{t("Given up")}</span>
            )}
            {g.status === "active" && g.results.length > 0 && (
              <span className={`xc-badge ${stateTone(g.state)}`}>
                {t(STATE_LABELS[g.state])}
              </span>
            )}
          </span>
        </div>
        <MoreMenu
          items={items}
          title={g.title}
          label={`${t("More")}: ${g.title}`}
        />
      </header>

      <div className="goals-progress">
        <strong className="goals-percent">{fmtPercent(g.progress)}</strong>
        <ProgressBar
          progress={g.progress}
          expected={g.expected}
          label={`${g.title} ${t("Goal progress")}`}
        />
        <small className="goals-pace">
          {g.progress == null
            ? t("No key result can be measured yet")
            : `${t("Goal progress")} ${fmtPercent(g.progress)} · ${t("Expected by now")} ${fmtPercent(g.expected)}`}
        </small>
      </div>

      {g.note && <p className="goals-note">{g.note}</p>}

      {g.results.length === 0 ? (
        <p className="goals-empty-line">
          {t("No key results yet. Edit the goal to add some.")}
        </p>
      ) : (
        <ul className="goals-results">
          {g.results.map((r) => (
            <ResultRow key={r.id} result={r} editable={g.status === "active"} />
          ))}
        </ul>
      )}
    </article>
  );
}

function ResultRow({
  result: r,
  editable,
}: {
  result: GoalResult;
  editable: boolean;
}) {
  const t = useT();
  const set = useSetResultValue();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const manual = r.source === "manual";

  const open = () => {
    setText(r.value == null ? "" : fmtNum(r.value));
    setEditing(true);
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = parseNumber(text);
    if (value == null)
      return toast({ message: t("Enter a number"), tone: "error" });
    set.mutate(
      { id: r.id, value },
      {
        onSuccess: () => {
          toast(t("Saved"));
          setEditing(false);
        },
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  return (
    <li className="goals-result">
      <div className="goals-result-main">
        <span className="goals-result-name">
          {r.name}
          {!manual && (
            <small>
              {t(SOURCE_LABELS[r.source])}
              {r.refLabel ? ` · ${r.refLabel}` : ""}
            </small>
          )}
        </span>
        <span className="goals-result-value">
          {r.unavailable ? (
            <small className="goals-unavailable">
              {t(UNAVAILABLE[r.unavailable] ?? "No data")}
            </small>
          ) : (
            <>
              <span>{valueText(r.value, r.target, r.unit)}</span>
              {r.state === "behind" && (
                <span className="xc-badge warn">{t(STATE_LABELS.behind)}</span>
              )}
            </>
          )}
        </span>
        {manual && editable && !editing && (
          <button type="button" className="xc-btn small" onClick={open}>
            {t("Update value")}
          </button>
        )}
      </div>
      {!r.unavailable && (
        <ProgressBar
          progress={r.progress}
          label={`${r.name} ${t("Goal progress")}`}
        />
      )}
      {editing && (
        <form className="goals-update" onSubmit={submit}>
          <input
            className="xc-input"
            inputMode="decimal"
            aria-label={`${r.name} ${t("Current value")}`}
            value={text}
            onChange={(e) => setText(e.target.value)}
            autoFocus
          />
          {r.unit && <small>{r.unit}</small>}
          <button
            type="submit"
            className="xc-btn primary small"
            disabled={set.isPending}
          >
            {t("Save")}
          </button>
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setEditing(false)}
          >
            {t("Cancel")}
          </button>
        </form>
      )}
    </li>
  );
}
