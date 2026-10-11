import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCreateSkill, useUpdateSkill, type Skill } from "./api";

/** 新建或编辑一项技能：名称和每周目标。 */
export default function SkillDialog({
  open,
  onClose,
  skill,
}: {
  open: boolean;
  onClose: () => void;
  skill?: Skill;
}) {
  const t = useT();
  const create = useCreateSkill();
  const update = useUpdateSkill();
  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    setName(skill?.name ?? "");
    setTarget(skill?.weeklyTarget ? String(skill.weeklyTarget) : "");
    setError("");
  }, [open, skill]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const weeklyTarget = target.trim() === "" ? 0 : Number(target);
    try {
      if (skill) {
        await update.mutateAsync({
          id: skill.id,
          body: { name: name.trim(), weeklyTarget },
        });
      } else {
        await create.mutateAsync({
          name: name.trim(),
          weeklyTarget: weeklyTarget || undefined,
        });
      }
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t(skill ? "Skill edit" : "Skill new")}
    >
      <form onSubmit={submit} className="sk-form">
        <label className="xc-field">
          <span>{t("Skill name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={50}
            placeholder={t("Skill name hint")}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Skill weekly target")}</span>
          <input
            className="xc-input"
            type="number"
            min={1}
            max={10080}
            value={target}
            placeholder={t("Skill weekly target hint")}
            onChange={(e) => setTarget(e.target.value)}
          />
        </label>
        {error && (
          <p className="xc-error" role="alert">
            {error}
          </p>
        )}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={create.isPending || update.isPending || !name.trim()}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
