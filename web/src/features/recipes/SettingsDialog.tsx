import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useHabitList } from "../habits/api";
import { useRecipeSettings, useSaveRecipeSettings, type DietGoal } from "./api";
import { addTag } from "./logic";

const GOALS: { value: DietGoal; label: string }[] = [
  { value: "none", label: "No goal" },
  { value: "lose", label: "Lose weight" },
  { value: "gain", label: "Build muscle" },
  { value: "steady", label: "Keep my weight" },
];

/** 菜谱设置（B210）：做完一顿在哪个习惯打卡、饮食目标、常备调料。 */
export default function SettingsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useRecipeSettings();
  const habits = useHabitList();
  const save = useSaveRecipeSettings();
  const [habitId, setHabitId] = useState<number | null>(null);
  const [goal, setGoal] = useState<DietGoal>("none");
  const [staples, setStaples] = useState<string[]>([]);
  const [text, setText] = useState("");

  useEffect(() => {
    if (!open || !settings.data) return;
    setHabitId(settings.data.habitId);
    setGoal(settings.data.dietGoal);
    setStaples(settings.data.staples);
    setText("");
  }, [open, settings.data]);

  const habitItems = (habits.data ?? []).filter(
    (h) => !h.archived || h.id === habitId,
  );

  const onSave = () =>
    save.mutate(
      { habitId, dietGoal: goal, staples: addTag(staples, text) },
      {
        onSuccess: () => {
          toast(t("Saved"));
          onClose();
        },
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Recipe settings")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={save.isPending || !settings.data}
            onClick={onSave}
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <label className="xc-field">
        <span>{t("Check in a habit after cooking")}</span>
        <select
          className="xc-select"
          value={habitId ?? ""}
          onChange={(e) =>
            setHabitId(e.target.value ? Number(e.target.value) : null)
          }
        >
          <option value="">{t("Do not check in")}</option>
          {habitItems.map((h) => (
            <option key={h.id} value={h.id}>
              {h.name}
            </option>
          ))}
        </select>
        <small>
          {t(
            "Each meal you record as cooked yourself adds one check-in. Deleting the record does not undo it.",
          )}
        </small>
      </label>
      <label className="xc-field">
        <span>{t("Diet goal")}</span>
        <select
          className="xc-select"
          value={goal}
          onChange={(e) => setGoal(e.target.value as DietGoal)}
        >
          {GOALS.map((g) => (
            <option key={g.value} value={g.value}>
              {t(g.label)}
            </option>
          ))}
        </select>
        <small>
          {t(
            "Suggestions use this goal. With no goal, a gain of more than 1 kg in 30 days counts as half a weight loss goal. The 1 kg line is a rule of thumb, not from a study.",
          )}
        </small>
      </label>
      <div className="xc-field">
        <span>{t("Pantry staples")}</span>
        <div className="recipes-tag-input">
          {staples.map((x) => (
            <span key={x} className="xc-badge">
              {x}
              <button
                type="button"
                aria-label={`${t("Remove")} ${x}`}
                onClick={() => setStaples(staples.filter((y) => y !== x))}
              >
                <X size={11} />
              </button>
            </span>
          ))}
          {staples.length < 50 && (
            <input
              className="xc-input"
              value={text}
              maxLength={20}
              placeholder={t("Add a staple. Press Enter")}
              aria-label={t("Pantry staples")}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  setStaples(addTag(staples, text));
                  setText("");
                }
              }}
            />
          )}
        </div>
        <small>
          {t("When you filter by what you have, these never count as missing.")}
        </small>
      </div>
    </Dialog>
  );
}
