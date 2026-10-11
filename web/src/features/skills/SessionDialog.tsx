import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useLogSession, useUpdateSession, type SkillSession } from "./api";

/** 记一次练习，或改一条已有的记录。 */
export default function SessionDialog({
  open,
  onClose,
  skillId,
  skillName,
  today,
  session,
}: {
  open: boolean;
  onClose: () => void;
  skillId: number;
  skillName: string;
  today?: string;
  session?: SkillSession;
}) {
  const t = useT();
  const log = useLogSession();
  const update = useUpdateSession();
  const [day, setDay] = useState("");
  const [minutes, setMinutes] = useState("");
  const [content, setContent] = useState("");
  const [stuck, setStuck] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    setDay(session?.practicedOn ?? today ?? "");
    setMinutes(session ? String(session.minutes) : "");
    setContent(session?.content ?? "");
    setStuck(session?.stuck ?? "");
    setError("");
  }, [open, session, today]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const body = {
      practicedOn: day || undefined,
      minutes: Number(minutes),
      content: content.trim(),
      stuck: stuck.trim(),
    };
    try {
      if (session) await update.mutateAsync({ id: session.id, body });
      else await log.mutateAsync({ skillId, body });
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
      title={`${t("Skill log title")} · ${skillName}`}
    >
      <form onSubmit={submit} className="sk-form">
        <div className="sk-row">
          <label className="xc-field">
            <span>{t("Date")}</span>
            <input
              className="xc-input"
              type="date"
              max={today}
              value={day}
              onChange={(e) => setDay(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Minutes")}</span>
            <input
              className="xc-input"
              type="number"
              min={1}
              max={1440}
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
            />
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Skill content")}</span>
          <textarea
            className="xc-textarea"
            rows={2}
            maxLength={1000}
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Skill stuck")}</span>
          <textarea
            className="xc-textarea"
            rows={2}
            maxLength={1000}
            value={stuck}
            placeholder={t("Skill stuck hint")}
            onChange={(e) => setStuck(e.target.value)}
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
            disabled={log.isPending || update.isPending || !minutes}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
