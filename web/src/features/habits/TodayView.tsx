import { useState, type ReactNode } from "react";
import { Bell, Flame, HeartPulse, Pencil, Plus, Undo2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatTime } from "../../lib/time";
import {
  useCheckin,
  useHabitsToday,
  useUndoCheckin,
  type Habit,
  type HabitToday,
} from "./api";
import HabitDialog from "./HabitDialog";
import { formatAmount, ratio, remindSummary, ringGeometry } from "./progress";

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

export function colorVar(color: string): string {
  return ["ok", "info", "warn", "danger", "accent"].includes(color)
    ? `var(--xc-${color})`
    : "var(--xc-accent)";
}

export function ProgressRing({
  value,
  color,
  size = 72,
  children,
}: {
  value: number;
  color: string;
  size?: number;
  children?: ReactNode;
}) {
  const stroke = 7;
  const r = (size - stroke) / 2;
  const { circumference, offset } = ringGeometry(value, r);
  return (
    <div className="habits-ring" style={{ width: size, height: size }}>
      <svg
        viewBox={`0 0 ${size} ${size}`}
        width={size}
        height={size}
        aria-hidden="true"
      >
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke="var(--xc-surface-2)"
          strokeWidth={stroke}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
          className="habits-ring-bar"
        />
      </svg>
      <div className="habits-ring-label">{children}</div>
    </div>
  );
}

export default function TodayView({
  creating,
  onCloseCreate,
}: {
  creating: boolean;
  onCloseCreate: () => void;
}) {
  const t = useT();
  const today = useHabitsToday();
  const [editing, setEditing] = useState<Habit | null>(null);
  return (
    <>
      {today.isPending ? (
        <Loading />
      ) : today.isError ? (
        <ErrorState error={today.error} onRetry={() => today.refetch()} />
      ) : today.data.length === 0 ? (
        <EmptyState title={t("No habits yet")} icon={<HeartPulse size={28} />}>
          <span>{t("Add one, for example drinking water or reading.")}</span>
        </EmptyState>
      ) : (
        <div className="habits-grid">
          {today.data.map((p) => (
            <HabitCard
              key={p.habit.id}
              progress={p}
              onEdit={() => setEditing(p.habit)}
            />
          ))}
        </div>
      )}
      <HabitDialog
        open={creating || editing !== null}
        habit={editing}
        onClose={() => {
          setEditing(null);
          onCloseCreate();
        }}
      />
    </>
  );
}

function HabitCard({
  progress: p,
  onEdit,
}: {
  progress: HabitToday;
  onEdit: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const checkin = useCheckin();
  const undo = useUndoCheckin();
  const h = p.habit;
  const color = colorVar(h.color);
  const last = p.logs[0];
  const summary = remindSummary(h, language);

  const plusOne = () =>
    checkin.mutate(
      { id: h.id },
      {
        onSuccess: (res) =>
          toast({
            message:
              res.today.reached && !p.reached
                ? t("Goal reached today")
                : t("Checked in"),
            subtitle: `${h.name} ${formatAmount(res.today.done)}/${formatAmount(h.dailyTarget)} ${h.unit}`,
            onUndo: () => undo.mutate(res.log.id, { onError }),
          }),
        onError,
      },
    );

  return (
    <div className={`xc-card habits-card${p.reached ? " is-reached" : ""}`}>
      <ProgressRing value={ratio(p.done, h.dailyTarget)} color={color}>
        <strong>{formatAmount(p.done)}</strong>
        <small>/ {formatAmount(h.dailyTarget)}</small>
      </ProgressRing>
      <div className="habits-card-main">
        <div className="habits-card-title">
          {h.icon && <span className="habits-icon">{h.icon}</span>}
          <span>{h.name}</span>
          <button
            className="xc-btn ghost small habits-edit"
            aria-label={t("Edit")}
            title={t("Edit")}
            onClick={onEdit}
          >
            <Pencil size={13} />
          </button>
        </div>
        <div className="habits-card-meta">
          <span>{h.unit}</span>
          {p.streak > 0 && (
            <span title={t("Streak")}>
              <Flame size={12} /> {p.streak} {t("days")}
            </span>
          )}
          {summary && (
            <span>
              <Bell size={12} /> {summary}
            </span>
          )}
        </div>
        <div className="habits-card-actions">
          <button
            className="xc-btn primary small"
            disabled={checkin.isPending}
            onClick={plusOne}
          >
            <Plus size={14} /> 1 {h.unit}
          </button>
          {last && (
            <button
              className="xc-btn ghost small"
              disabled={undo.isPending}
              title={`${t("Undo")} ${formatTime(last.at, language)} +${formatAmount(last.amount)}`}
              onClick={() =>
                undo.mutate(last.id, {
                  onSuccess: () => toast(t("Undone")),
                  onError,
                })
              }
            >
              <Undo2 size={13} /> {t("Undo")}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
