import { useState, type ReactNode } from "react";
import MoreMenu from "../../components/ui/MoreMenu";
import {
  Bell,
  Check,
  Dumbbell,
  Flame,
  HeartPulse,
  Pencil,
  Plus,
  Undo2,
  X,
} from "lucide-react";
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
  type HabitTemplate,
  type HabitToday,
} from "./api";
import { HABIT_TEMPLATES } from "./presence";
import { FitnessSummary, TodayLogDialog } from "./FitnessModule";
import HabitDialog from "./HabitDialog";
import { formatAmount, ratio, remindSummary, ringGeometry } from "./progress";
import { relativeTime } from "../../lib/time";

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
  // B96：从推荐模板新建
  const [fromTemplate, setFromTemplate] = useState<HabitTemplate>();
  const active = (today.data ?? []).map((p) => p.habit);
  const existing = active
    .map((h) => h.template)
    .filter((x): x is HabitTemplate => !!x);
  return (
    <>
      {today.data && (
        <Suggestions
          habits={active}
          onCreate={setFromTemplate}
          onEdit={setEditing}
        />
      )}
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
      <HabitModules />
      <HabitDialog
        open={creating || editing !== null || fromTemplate !== undefined}
        habit={editing}
        initialTemplate={fromTemplate}
        existingTemplates={existing}
        onClose={() => {
          setEditing(null);
          setFromTemplate(undefined);
          onCloseCreate();
        }}
      />
    </>
  );
}

/**
 * B96：推荐模板。没建过的点了打开新建弹窗并填好；建过的显示“已添加”，
 * 点了打开那个习惯的编辑，不再新建。全部建过时不显示。
 */
function Suggestions({
  habits,
  onCreate,
  onEdit,
}: {
  habits: Habit[];
  onCreate: (id: HabitTemplate) => void;
  onEdit: (habit: Habit) => void;
}) {
  const t = useT();
  const byTemplate = new Map(
    habits.filter((h) => h.template).map((h) => [h.template!, h]),
  );
  if (HABIT_TEMPLATES.every((tpl) => byTemplate.has(tpl.id))) return null;
  return (
    <section className="habits-suggest" aria-label={t("Suggested")}>
      <span className="habits-suggest-label">{t("Suggested")}</span>
      {HABIT_TEMPLATES.map((tpl) => {
        const habit = byTemplate.get(tpl.id);
        return habit ? (
          <button
            key={tpl.id}
            type="button"
            className="xc-btn small ghost habits-suggest-added"
            title={t("Already added. Click to edit.")}
            onClick={() => onEdit(habit)}
          >
            <span aria-hidden>{tpl.input.icon}</span> {t(tpl.label)}
            <Check size={13} />
          </button>
        ) : (
          <button
            key={tpl.id}
            type="button"
            className="xc-btn small"
            onClick={() => onCreate(tpl.id)}
          >
            <Plus size={13} />
            <span aria-hidden>{tpl.input.icon}</span> {t(tpl.label)}
          </button>
        );
      })}
    </section>
  );
}

/** 习惯页下面可以加的模块。只有健身一种，以后加别的也放这里。 */
const modules = [
  { id: "fitness", title: "Workout", icon: <Dumbbell size={15} /> },
];
const MODULES_KEY = "xc.habits.modules";

function loadModules(): string[] {
  try {
    const raw = localStorage.getItem(MODULES_KEY);
    if (raw) return JSON.parse(raw) as string[];
  } catch {
    // 读不到就用默认
  }
  return ["fitness"];
}

function HabitModules() {
  const t = useT();
  const [enabled, setEnabled] = useState(loadModules);
  const save = (next: string[]) => {
    setEnabled(next);
    try {
      localStorage.setItem(MODULES_KEY, JSON.stringify(next));
    } catch {
      // 存不了就只在这次打开的页面里生效
    }
  };
  const available = modules.filter((m) => !enabled.includes(m.id));
  return (
    <>
      {modules
        .filter((m) => enabled.includes(m.id))
        .map((m) => (
          <section className="xc-card habits-module" key={m.id}>
            <div className="xc-card-head">
              <h2>
                {m.icon} {t(m.title)}
              </h2>
              <MoreMenu
                label={t("More")}
                items={[
                  {
                    key: "remove",
                    label: t("Remove module"),
                    icon: <X size={14} />,
                    onSelect: () => save(enabled.filter((id) => id !== m.id)),
                  },
                ]}
              />
            </div>
            <FitnessSummary />
          </section>
        ))}
      {available.length > 0 && (
        <div className="habits-add-module">
          {available.map((m) => (
            <button
              key={m.id}
              className="xc-btn small ghost"
              onClick={() => save([...enabled, m.id])}
            >
              <Plus size={14} /> {t("Add module")}：{t(m.title)}
            </button>
          ))}
        </div>
      )}
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
  const isWorkout = h.kind === "workout";
  const [logging, setLogging] = useState(false);

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
              {h.nextRemindAt && !p.reached && (
                <>
                  {" · "}
                  {t("Next reminder at")}{" "}
                  {relativeTime(h.nextRemindAt, language)}
                </>
              )}
            </span>
          )}
        </div>
        <div className="habits-card-actions">
          {isWorkout ? (
            <button
              className="xc-btn primary small"
              onClick={() => setLogging(true)}
            >
              <Dumbbell size={14} /> {t("Log workout")}
            </button>
          ) : (
            <button
              className="xc-btn primary small"
              disabled={checkin.isPending}
              onClick={plusOne}
            >
              <Plus size={14} /> 1 {h.unit}
            </button>
          )}
          {last && last.source !== "workout" && (
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
      {isWorkout && (
        <TodayLogDialog open={logging} onClose={() => setLogging(false)} />
      )}
    </div>
  );
}
