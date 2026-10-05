import { useEffect, useState, type FormEvent } from "react";
import { Archive, ArchiveRestore, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateHabit,
  useDeleteHabit,
  useUpdateHabit,
  type Habit,
  type HabitInput,
  type HabitKind,
  type HabitTemplate,
  type RemindMode,
  type RemindWhen,
  usePresence,
} from "./api";
import { HABIT_TEMPLATES, presenceText, presenceTone } from "./presence";
import { joinWindow, parseTimes, splitWindow } from "./progress";
import { confirmAction } from "../../components/ui/ConfirmDialog";

/** 常用图标，也可以自己输入一个。 */
export const iconChoices = [
  "💧",
  "🏃",
  "📖",
  "🧘",
  "🌙",
  "💪",
  "🥗",
  "🍎",
  "🚶",
  "🚴",
  "🏊",
  "😴",
  "✍️",
  "🎸",
  "💊",
  "🦷",
  "☀️",
  "🧹",
  "💻",
  "🗣️",
  "📵",
  "🚭",
  "💰",
  "🙏",
];

const colors = ["accent", "ok", "info", "warn", "danger"];
const colorLabels: Record<string, string> = {
  accent: "Orange",
  ok: "Green",
  info: "Blue",
  warn: "Yellow",
  danger: "Red",
};

interface Props {
  open: boolean;
  onClose: () => void;
  habit?: Habit | null;
  /** B96：新建时先填好这个模板 */
  initialTemplate?: HabitTemplate;
  /** B96：已经建过的模板，不再重复建 */
  existingTemplates?: HabitTemplate[];
  /** 推荐习惯、健身方案：新建时先填好这些值（2026-10-05） */
  initialInput?: Omit<HabitInput, "activeHostIds">;
}

export default function HabitDialog({
  open,
  onClose,
  habit,
  initialTemplate,
  existingTemplates = [],
  initialInput,
}: Props) {
  const t = useT();
  const create = useCreateHabit();
  const update = useUpdateHabit();
  const remove = useDeleteHabit();
  const [name, setName] = useState("");
  const [icon, setIcon] = useState("");
  const [kind, setKind] = useState<HabitKind>("count");
  const [picking, setPicking] = useState(false);
  const [color, setColor] = useState("accent");
  const [unit, setUnit] = useState("次");
  const [target, setTarget] = useState("1");
  const [mode, setMode] = useState<RemindMode>("none");
  const [interval, setIntervalMinutes] = useState("60");
  const [windowStart, setWindowStart] = useState("09:00");
  const [windowEnd, setWindowEnd] = useState("21:00");
  const [times, setTimes] = useState("");
  const [entity, setEntity] = useState("");
  const [error, setError] = useState("");
  // B83：什么时候提醒、看哪几台电脑、要不要在电脑上弹通知、用了哪个模板
  const [when, setWhen] = useState<RemindWhen[]>(["window"]);
  const [hostIds, setHostIds] = useState<string[]>([]);
  const [onHost, setOnHost] = useState(false);
  const [template, setTemplate] = useState<HabitTemplate | undefined>();
  const presence = usePresence(open);

  useEffect(() => {
    if (!open) return;
    setError("");
    const h = habit;
    setName(h?.name ?? "");
    setIcon(h?.icon ?? "");
    setKind(h?.kind ?? "count");
    setPicking(false);
    setColor(h?.color || "accent");
    setUnit(h?.unit ?? "次");
    setTarget(String(h?.dailyTarget ?? 1));
    setMode(h?.remindMode ?? "none");
    setIntervalMinutes(String(h?.remindIntervalMinutes || 60));
    const w = splitWindow(h?.remindWindow ?? "09:00-21:00");
    setWindowStart(w.start);
    setWindowEnd(w.end);
    setTimes((h?.remindTimes ?? []).join(", "));
    setEntity(h?.haEntityId ?? "");
    setWhen(h?.remindWhen?.length ? h.remindWhen : ["window"]);
    setHostIds(h?.activeHostIds ?? []);
    setOnHost(h?.remindOnHost ?? false);
    setTemplate(h?.template);
    if (!h && initialTemplate) applyTemplate(initialTemplate);
    else if (!h && initialInput) applyInput(initialInput);
    // applyTemplate 只在打开时用一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, habit, initialTemplate, initialInput]);

  const applyTemplate = (id: HabitTemplate) => {
    const tpl = HABIT_TEMPLATES.find((x) => x.id === id);
    if (!tpl) return;
    setTemplate(id);
    applyInput(tpl.input);
  };

  const applyInput = (v: Omit<HabitInput, "activeHostIds">) => {
    setKind(v.kind ?? "count");
    setName(v.name);
    setIcon(v.icon ?? "");
    setUnit(v.unit ?? "次");
    setTarget(String(v.dailyTarget ?? 1));
    setMode(v.remindMode ?? "none");
    setIntervalMinutes(String(v.remindIntervalMinutes || 60));
    setTimes((v.remindTimes ?? []).join(", "));
    setWhen(v.remindWhen ?? ["window"]);
    // 只有一台电脑时直接选上
    const hosts = presence.data ?? [];
    if (v.remindWhen?.includes("active") && hosts.length === 1)
      setHostIds([hosts[0].hostId]);
  };

  // “时间窗内”单独用；“醒着”和“工作时间”二选一；“在用电脑时”可以和它们一起勾。
  const toggleWhen = (w: RemindWhen) => {
    setWhen((prev) => {
      let next: RemindWhen[];
      if (prev.includes(w)) next = prev.filter((x) => x !== w);
      else if (w === "window") next = ["window"];
      else if (w === "active")
        next = [...prev.filter((x) => x !== "window"), "active"];
      else next = [...prev.filter((x) => x === "active"), w];
      return next.length ? next : ["window"];
    });
  };

  const pending = create.isPending || update.isPending;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const dailyTarget = Number(target);
    if (!name.trim()) return setError(t("Name is required"));
    if (!(dailyTarget > 0))
      return setError(t("The daily goal must be above 0"));
    const remindTimes = parseTimes(times);
    if (mode === "times" && (!remindTimes || remindTimes.length === 0))
      return setError(t("Write times like 08:00, 20:00"));
    const minutes = Number(interval);
    if (mode === "interval" && !(minutes >= 5 && minutes <= 1440))
      return setError(t("The interval must be 5 to 1440 minutes"));
    const active = mode === "interval" && when.includes("active");
    if (active && hostIds.length === 0)
      return setError(t("Pick at least one computer"));
    const body: HabitInput = {
      name: name.trim(),
      icon: icon.trim(),
      kind,
      color,
      unit: kind === "workout" ? "次" : unit.trim() || "次",
      dailyTarget,
      remindMode: mode,
      remindIntervalMinutes: mode === "interval" ? minutes : 0,
      remindWindow:
        mode === "interval" && when.includes("window")
          ? joinWindow(windowStart, windowEnd)
          : "",
      remindTimes: mode === "times" ? (remindTimes ?? []) : [],
      remindWhen: mode === "interval" ? when : ["window"],
      activeHostIds: active ? hostIds : [],
      remindOnHost: active && onHost,
      template,
      haEntityId: entity.trim(),
    };
    try {
      if (habit) await update.mutateAsync({ id: habit.id, body });
      else await create.mutateAsync(body);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const archive = async () => {
    if (!habit) return;
    try {
      await update.mutateAsync({
        id: habit.id,
        body: { archived: !habit.archived },
      });
      toast(habit.archived ? t("Restored") : t("Archived"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const destroy = async () => {
    if (
      !habit ||
      !(await confirmAction({
        title: t("Delete this habit and all its check-ins?"),
      }))
    )
      return;
    try {
      await remove.mutateAsync(habit.id);
      toast(t("Deleted"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={habit ? t("Edit habit") : t("New habit")}
    >
      <form onSubmit={submit}>
        {!habit && (
          <div
            className="habits-templates"
            role="group"
            aria-label={t("Templates")}
          >
            {HABIT_TEMPLATES.map((tpl) => {
              const added = existingTemplates.includes(tpl.id);
              return (
                <button
                  key={tpl.id}
                  type="button"
                  className={`xc-btn small${template === tpl.id ? " on" : ""}`}
                  aria-pressed={template === tpl.id}
                  disabled={added}
                  title={added ? t("Template already added") : undefined}
                  onClick={() => applyTemplate(tpl.id)}
                >
                  <span aria-hidden>{tpl.input.icon}</span> {t(tpl.label)}
                  {added && ` · ${t("Template added")}`}
                </button>
              );
            })}
          </div>
        )}
        <div className="habits-kind">
          <Segmented<HabitKind>
            label={t("Kind")}
            value={kind}
            onChange={(v) => {
              setKind(v);
              if (v === "workout") {
                setTarget("1");
                if (!icon) setIcon("💪");
              }
            }}
            options={[
              { value: "count", label: t("Check-in habit") },
              { value: "workout", label: t("Workout habit") },
            ]}
          />
        </div>
        {kind === "workout" && (
          <p className="habits-kind-hint habits-kind">
            {t(
              "Logging a workout checks this habit in. You can pick the exercises when logging.",
            )}
          </p>
        )}
        <div className="habits-form-row habits-form-name">
          <div className="xc-field">
            <span>{t("Icon")}</span>
            <button
              type="button"
              className={`habits-icon-pick${picking ? " open" : ""}`}
              aria-expanded={picking}
              aria-label={t("Choose icon")}
              onClick={() => setPicking(!picking)}
            >
              {icon || "＋"}
            </button>
          </div>
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如 喝水"
              maxLength={100}
              autoFocus
            />
          </label>
        </div>
        {picking && (
          <div
            className="habits-icon-grid"
            role="group"
            aria-label={t("Choose icon")}
          >
            {iconChoices.map((c) => (
              <button
                type="button"
                key={c}
                className={c === icon ? "on" : ""}
                aria-pressed={c === icon}
                onClick={() => {
                  setIcon(c);
                  setPicking(false);
                }}
              >
                {c}
              </button>
            ))}
            <input
              className="xc-input"
              value={iconChoices.includes(icon) ? "" : icon}
              onChange={(e) => setIcon(e.target.value)}
              maxLength={4}
              placeholder={t("Other")}
              aria-label={t("Custom icon")}
            />
            {icon && (
              <button
                type="button"
                className="habits-icon-clear"
                onClick={() => setIcon("")}
              >
                {t("No icon")}
              </button>
            )}
          </div>
        )}
        <div className="habits-form-row">
          <label className="xc-field">
            <span>{t("Daily goal")}</span>
            <input
              className="xc-input"
              type="number"
              min="0"
              step="any"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            />
          </label>
          {kind !== "workout" && (
            <label className="xc-field">
              <span>{t("Unit")}</span>
              <input
                className="xc-input"
                value={unit}
                onChange={(e) => setUnit(e.target.value)}
                placeholder="杯、分钟、次"
              />
            </label>
          )}
          <label className="xc-field">
            <span>{t("Color")}</span>
            <select
              className="xc-select"
              value={color}
              onChange={(e) => setColor(e.target.value)}
            >
              {colors.map((c) => (
                <option key={c} value={c}>
                  {t(colorLabels[c])}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Reminders")}</span>
          <select
            className="xc-select"
            value={mode}
            onChange={(e) => setMode(e.target.value as RemindMode)}
          >
            <option value="none">{t("No reminders")}</option>
            <option value="interval">{t("Every few minutes")}</option>
            <option value="times">{t("At set times")}</option>
          </select>
        </label>
        {mode === "interval" && (
          <div className="xc-field">
            <span>{t("When to remind")}</span>
            <div
              className="habits-when"
              role="group"
              aria-label={t("When to remind")}
            >
              {(
                [
                  ["window", "In a time window"],
                  ["awake", "While I'm awake"],
                  ["work", "During work hours"],
                  ["active", "While I'm at the computer"],
                ] as [RemindWhen, string][]
              ).map(([w, label]) => (
                <label key={w} className="xc-check">
                  <input
                    type="checkbox"
                    checked={when.includes(w)}
                    onChange={() => toggleWhen(w)}
                  />
                  {t(label)}
                </label>
              ))}
            </div>
            <small>
              {when.includes("active")
                ? t(
                    "Counts continuous use. Leaving the computer resets the timer.",
                  )
                : when.includes("awake") || when.includes("work")
                  ? t("Uses your daily schedule. It can cross midnight.")
                  : ""}
            </small>
          </div>
        )}
        {mode === "interval" && when.includes("active") && (
          <div className="xc-field">
            <span>{t("Computers to watch")}</span>
            {presence.isPending ? (
              <small className="xc-muted">{t("Loading")}…</small>
            ) : (presence.data ?? []).length === 0 ? (
              <small className="xc-muted">{t("No computers yet")}</small>
            ) : (
              <div className="habits-hosts">
                {(presence.data ?? []).map((p) => (
                  <label key={p.hostId} className="xc-check">
                    <input
                      type="checkbox"
                      checked={hostIds.includes(p.hostId)}
                      onChange={(e) =>
                        setHostIds((ids) =>
                          e.target.checked
                            ? [...ids, p.hostId]
                            : ids.filter((x) => x !== p.hostId),
                        )
                      }
                    />
                    <span className="habits-host-name">{p.name}</span>
                    <span className={`xc-badge ${presenceTone(p.state)}`}>
                      {presenceText(p, t)}
                    </span>
                  </label>
                ))}
              </div>
            )}
            <label className="xc-check">
              <input
                type="checkbox"
                checked={onHost}
                onChange={(e) => setOnHost(e.target.checked)}
              />
              {t("Also pop up a notification on that computer")}
            </label>
          </div>
        )}
        {mode === "interval" && (
          <div className="habits-form-row">
            <label className="xc-field">
              <span>
                {when.includes("active")
                  ? t("After using it for (minutes)")
                  : t("Every (minutes)")}
              </span>
              <input
                className="xc-input"
                type="number"
                min="5"
                max="1440"
                value={interval}
                onChange={(e) => setIntervalMinutes(e.target.value)}
              />
            </label>
            {when.includes("window") && (
              <>
                <label className="xc-field">
                  <span>{t("From")}</span>
                  <input
                    className="xc-input"
                    type="time"
                    value={windowStart}
                    onChange={(e) => setWindowStart(e.target.value)}
                  />
                </label>
                <label className="xc-field">
                  <span>{t("To")}</span>
                  <input
                    className="xc-input"
                    type="time"
                    value={windowEnd}
                    onChange={(e) => setWindowEnd(e.target.value)}
                  />
                </label>
              </>
            )}
          </div>
        )}
        {mode === "times" && (
          <label className="xc-field">
            <span>{t("Times")}</span>
            <input
              className="xc-input"
              value={times}
              onChange={(e) => setTimes(e.target.value)}
              placeholder="08:00, 20:00"
            />
          </label>
        )}
        {mode !== "none" && (
          <p className="xc-muted habits-hint">
            {t("Reminders stop once today's goal is reached.")}
          </p>
        )}
        <label className="xc-field">
          <span>
            {t("Home Assistant entity")}{" "}
            <small className="xc-muted">· {t("optional")}</small>
          </span>
          <input
            className="xc-input xc-mono"
            value={entity}
            onChange={(e) => setEntity(e.target.value)}
            placeholder="binary_sensor.toothbrush"
          />
          <small>
            {t("Each state change of this entity counts as one check-in.")}
          </small>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions habits-dialog-actions">
          {habit && (
            <>
              <button
                type="button"
                className="xc-btn ghost small"
                onClick={archive}
              >
                {habit.archived ? (
                  <ArchiveRestore size={14} />
                ) : (
                  <Archive size={14} />
                )}{" "}
                {habit.archived ? t("Restore") : t("Archive")}
              </button>
              <button
                type="button"
                className="xc-btn danger small"
                onClick={destroy}
              >
                <Trash2 size={14} /> {t("Delete")}
              </button>
              <span className="xc-spacer" />
            </>
          )}
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={pending}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
