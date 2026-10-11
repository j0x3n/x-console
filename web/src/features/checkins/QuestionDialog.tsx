import { useEffect, useState, type FormEvent } from "react";
import { Plus, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useSaveCheckinQuestion, type CheckinQuestion } from "./api";

const CLOCK = /^([01]\d|2[0-3]):[0-5]\d$/;

/** 新建或修改一个打卡问题：问法、1 分和 5 分的说明、每天问的时间。 */
export default function QuestionDialog({
  open,
  onClose,
  question,
}: {
  open: boolean;
  onClose: () => void;
  question?: CheckinQuestion | null;
}) {
  const t = useT();
  const save = useSaveCheckinQuestion();
  const [label, setLabel] = useState("");
  const [low, setLow] = useState("");
  const [high, setHigh] = useState("");
  const [times, setTimes] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setLabel(question?.label ?? "");
    setLow(question?.lowLabel ?? "");
    setHigh(question?.highLabel ?? "");
    setTimes(question?.times ?? ["10:00"]);
    setEnabled(question?.enabled ?? true);
  }, [open, question]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!label.trim()) return setError(t("Please enter the question"));
    if (times.some((x) => !CLOCK.test(x)))
      return setError(t("Every time must look like 10:00"));
    try {
      await save.mutateAsync({
        id: question?.id,
        body: {
          label: label.trim(),
          lowLabel: low.trim(),
          highLabel: high.trim(),
          times,
          enabled,
        },
      });
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
      title={question ? t("Edit question") : t("New question")}
    >
      <form onSubmit={submit} className="checkins-form">
        <label className="xc-field">
          <span>{t("Question")}</span>
          <input
            className="xc-input"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            maxLength={60}
            autoFocus
            placeholder="例如 现在心情怎么样"
          />
        </label>
        <div className="checkins-form-row">
          <label className="xc-field">
            <span>{t("What 1 means")}</span>
            <input
              className="xc-input"
              value={low}
              onChange={(e) => setLow(e.target.value)}
              maxLength={30}
              placeholder="很低"
            />
          </label>
          <label className="xc-field">
            <span>{t("What 5 means")}</span>
            <input
              className="xc-input"
              value={high}
              onChange={(e) => setHigh(e.target.value)}
              maxLength={30}
              placeholder="很好"
            />
          </label>
        </div>
        <div className="xc-field">
          <span>{t("Times to ask")}</span>
          <div className="checkins-times">
            {times.map((value, i) => (
              <span key={i} className="checkins-time">
                <input
                  className="xc-input"
                  type="time"
                  value={value}
                  aria-label={`${t("Times to ask")} ${i + 1}`}
                  onChange={(e) =>
                    setTimes((list) =>
                      list.map((x, j) => (j === i ? e.target.value : x)),
                    )
                  }
                />
                <button
                  type="button"
                  className="icon-button"
                  aria-label={t("Remove time")}
                  title={t("Remove time")}
                  onClick={() =>
                    setTimes((list) => list.filter((_, j) => j !== i))
                  }
                >
                  <X size={14} />
                </button>
              </span>
            ))}
            {times.length < 6 && (
              <button
                type="button"
                className="xc-btn small"
                onClick={() => setTimes((list) => [...list, "12:00"])}
              >
                <Plus size={14} /> {t("Add ask time")}
              </button>
            )}
          </div>
        </div>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          <span>{t("Ask this question")}</span>
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
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
