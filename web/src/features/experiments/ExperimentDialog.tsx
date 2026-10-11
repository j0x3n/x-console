import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  toInput,
  useExperimentMetrics,
  useSaveExperiment,
  type Experiment,
  type ExperimentMetricOption,
} from "./api";

const GROUPS: Array<{ id: ExperimentMetricOption["group"]; label: string }> = [
  { id: "body", label: "Experiment body data" },
  { id: "checkin", label: "Experiment self-rated" },
  { id: "habit", label: "Experiment habits" },
  { id: "focus", label: "Experiment focus" },
  { id: "screen", label: "Experiment computer time" },
];

const iso = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const shift = (days: number) => {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return iso(d);
};

/** 新建或修改一个实验。 */
export default function ExperimentDialog({
  open,
  onClose,
  experiment,
}: {
  open: boolean;
  onClose: () => void;
  experiment?: Experiment | null;
}) {
  const t = useT();
  const save = useSaveExperiment();
  const catalog = useExperimentMetrics(open);
  const [title, setTitle] = useState("");
  const [change, setChange] = useState("");
  const [baselineStart, setBaselineStart] = useState(shift(0));
  const [changeStart, setChangeStart] = useState(shift(14));
  const [endDate, setEndDate] = useState(shift(27));
  const [note, setNote] = useState("");
  const [metrics, setMetrics] = useState<string[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setTitle(experiment?.title ?? "");
    setChange(experiment?.change ?? "");
    setBaselineStart(experiment?.baselineStart ?? shift(0));
    setChangeStart(experiment?.changeStart ?? shift(14));
    setEndDate(experiment?.endDate ?? shift(27));
    setNote(experiment?.note ?? "");
    setMetrics(experiment?.metrics.map((m) => m.key) ?? []);
  }, [open, experiment]);

  const options = catalog.data?.items ?? [];
  // 已有实验的指标现在选不了（模块被锁）时也留着，免得保存时丢掉
  const keep = (experiment?.metrics ?? []).filter(
    (m) => !options.some((o) => o.key === m.key),
  );
  const toggle = (key: string) =>
    setMetrics((list) =>
      list.includes(key) ? list.filter((k) => k !== key) : [...list, key],
    );

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim()) return setError(t("Please enter the experiment name"));
    if (metrics.length === 0) return setError(t("Pick at least one metric"));
    if (metrics.length > 6) return setError(t("Pick at most 6 metrics"));
    try {
      await save.mutateAsync({
        id: experiment?.id,
        body: {
          ...(experiment ? toInput(experiment) : {}),
          title: title.trim(),
          change: change.trim(),
          baselineStart,
          changeStart,
          endDate,
          note,
          metrics,
        },
      });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const selectedSubjective = metrics.some(
    (k) => options.find((o) => o.key === k)?.subjective,
  );

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={experiment ? t("Edit experiment") : t("New experiment")}
      wide
    >
      <form onSubmit={submit} className="experiments-form">
        <label className="xc-field">
          <span>{t("Experiment name")}</span>
          <input
            className="xc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            maxLength={100}
            autoFocus
            placeholder="例如 下午不喝咖啡"
          />
        </label>
        <label className="xc-field">
          <span>{t("What you change")}</span>
          <textarea
            className="xc-textarea"
            rows={2}
            value={change}
            onChange={(e) => setChange(e.target.value)}
            maxLength={5000}
            placeholder="例如 下午 2 点以后不喝咖啡和茶"
          />
        </label>
        <div className="experiments-dates">
          <label className="xc-field">
            <span>{t("Baseline starts")}</span>
            <input
              className="xc-input"
              type="date"
              value={baselineStart}
              onChange={(e) => setBaselineStart(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Change starts")}</span>
            <input
              className="xc-input"
              type="date"
              value={changeStart}
              onChange={(e) => setChangeStart(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Experiment ends")}</span>
            <input
              className="xc-input"
              type="date"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
            />
          </label>
        </div>
        <p className="xc-muted experiments-hint">
          {t(
            "Before the change date you carry on as usual (baseline). From that day you make the change. Each phase needs at least 3 days; 14 or more is better.",
          )}
        </p>
        <fieldset className="experiments-metrics">
          <legend>{t("Numbers to watch (up to 6)")}</legend>
          {catalog.isPending && <p className="xc-muted">{t("Loading")}</p>}
          {GROUPS.map((g) => {
            const list = options.filter((o) => o.group === g.id);
            if (list.length === 0) return null;
            return (
              <div key={g.id} className="experiments-group">
                <strong>{t(g.label)}</strong>
                {list.map((o) => (
                  <label key={o.key} className="xc-check">
                    <input
                      type="checkbox"
                      checked={metrics.includes(o.key)}
                      onChange={() => toggle(o.key)}
                    />
                    <span>{o.label}</span>
                  </label>
                ))}
              </div>
            );
          })}
          {keep.length > 0 && (
            <div className="experiments-group">
              <strong>{t("Not available now")}</strong>
              {keep.map((m) => (
                <label key={m.key} className="xc-check">
                  <input
                    type="checkbox"
                    checked={metrics.includes(m.key)}
                    onChange={() => toggle(m.key)}
                  />
                  <span>{m.label}</span>
                </label>
              ))}
            </div>
          )}
        </fieldset>
        {selectedSubjective && (
          <p className="xc-muted experiments-hint">
            {t(
              "Self-rated scores are easy to bend with expectations. Prefer the body data when you can.",
            )}
          </p>
        )}
        <label className="xc-field">
          <span>{t("Experiment note")}</span>
          <textarea
            className="xc-textarea"
            rows={2}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            maxLength={5000}
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
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
