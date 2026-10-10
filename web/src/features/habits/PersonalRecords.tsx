import { useState } from "react";
import { Save } from "lucide-react";
import PageActions from "../../components/layout/PageActions";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { EmptyState } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import BodyTrend from "./BodyTrend";
import {
  useSavePersonalDay,
  type PersonalDay,
  type PersonalDayInput,
} from "./personalApi";

export function PersonalNote({
  day,
  field,
}: {
  day: PersonalDay;
  field: "english" | "food";
}) {
  const t = useT();
  const [value, setValue] = useState(day[field]);
  const save = useSavePersonalDay(day.date);
  const label = t(
    field === "english" ? "English expression notes" : "Food notes",
  );
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{label}</h2>
        <button
          className="xc-btn small"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              { [field]: value },
              { onSuccess: () => toast(t("Saved")) },
            )
          }
        >
          <Save size={14} /> {t("Save")}
        </button>
      </div>
      <div className="xc-field">
        <MarkdownEditor
          value={value}
          onChange={setValue}
          label={label}
          minRows={4}
        />
      </div>
    </section>
  );
}

export default function PersonalRecords({
  day,
  days,
  onDate,
}: {
  day: PersonalDay;
  days: PersonalDay[];
  onDate: (date: string) => void;
}) {
  const t = useT();
  const [form, setForm] = useState<PersonalDayInput>({
    weight: day.weight,
    waist: day.waist,
    sleep: day.sleep,
    steps: day.steps,
    restingHr: day.restingHr,
    energy: day.energy,
    back: day.back,
    note: day.note,
  });
  const save = useSavePersonalDay(day.date);
  const set = (key: keyof PersonalDayInput, value: string) =>
    setForm((p) => ({ ...p, [key]: value }));
  const submit = () =>
    save.mutate(form, { onSuccess: () => toast(t("Saved")) });
  return (
    <>
      <PageActions>
        <button
          className="xc-btn primary"
          disabled={save.isPending}
          onClick={submit}
          title={t("Save daily record")}
        >
          <Save size={15} /> {t("Save daily record")}
        </button>
      </PageActions>
      <div className="habits-personal-grid">
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Daily measurements")}</h2>
            <span className="xc-muted">{day.date}</span>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <div className="habits-personal-form">
              {(
                [
                  ["weight", "Body weight (kg)", 30, 250, "0.1"],
                  ["waist", "Waist (cm)", 40, 200, "0.1"],
                  ["sleep", "Sleep (hours)", 0, 24, "0.1"],
                  ["steps", "Actual steps", 0, 100000, "1"],
                  ["restingHr", "Resting heart rate (bpm)", 20, 220, "1"],
                ] as const
              ).map(([key, label, min, max, step]) => (
                <label className="xc-field" key={key}>
                  <span>{t(label)}</span>
                  <input
                    className="xc-input"
                    type="number"
                    min={min}
                    max={max}
                    step={step}
                    value={form[key] ?? ""}
                    onChange={(e) => set(key, e.target.value)}
                  />
                </label>
              ))}
              <label className="xc-field">
                <span>{t("Energy today")}</span>
                <select
                  className="xc-select"
                  value={form.energy ?? ""}
                  onChange={(e) => set("energy", e.target.value)}
                >
                  <option value="">{t("Not recorded")}</option>
                  {["充足", "一般", "疲劳"].map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </label>
              <label className="xc-field">
                <span>{t("Back response")}</span>
                <select
                  className="xc-select"
                  value={form.back ?? ""}
                  onChange={(e) => set("back", e.target.value)}
                >
                  <option value="">{t("Not recorded")}</option>
                  {[
                    "和平时相近",
                    "更舒服",
                    "酸胀增加",
                    "出现新放射感或麻木",
                  ].map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </label>
            </div>
            <div className="xc-field">
              <span>{t("Training and recovery notes")}</span>
              <MarkdownEditor
                value={form.note ?? ""}
                onChange={(v) => set("note", v)}
                label={t("Training and recovery notes")}
                minRows={4}
              />
            </div>
            <button
              className="habits-submit-hidden"
              type="submit"
              tabIndex={-1}
              aria-hidden="true"
            />
          </form>
        </section>
        <BodyTrend days={days} end={day.date} />
      </div>
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Personal record history")}</h2>
        </div>
        {days.length ? (
          <div className="xc-table-wrap">
            <table className="xc-table">
              <thead>
                <tr>
                  <th>{t("Date")}</th>
                  <th>{t("Body weight (kg)")}</th>
                  <th>{t("Sleep (hours)")}</th>
                  <th>{t("Actual steps")}</th>
                  <th>{t("Resting heart rate (bpm)")}</th>
                  <th>{t("Back response")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {days.slice(0, 60).map((d) => (
                  <tr key={d.date}>
                    <td>{d.date}</td>
                    <td>{d.weight || "—"}</td>
                    <td>{d.sleep || "—"}</td>
                    <td>{d.steps || "—"}</td>
                    <td>{d.restingHr || "—"}</td>
                    <td>{d.back || "—"}</td>
                    <td>
                      <button
                        className="xc-btn ghost small"
                        onClick={() => onDate(d.date)}
                      >
                        {t("View")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <EmptyState title={t("No personal records yet")} />
        )}
      </section>
    </>
  );
}
