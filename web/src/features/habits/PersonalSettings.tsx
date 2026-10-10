import { useRef, useState } from "react";
import { Download, Save, Upload } from "lucide-react";
import PageActions from "../../components/layout/PageActions";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import {
  exportPersonalBackup,
  useImportPersonalBackup,
  useSavePersonalProfile,
  type PersonalBackup,
  type PersonalLibrary,
  type PersonalProfile,
  type PersonalProfileInput,
} from "./personalApi";
import { dateKey } from "./personal";
import BodySync from "./BodySync";

export default function PersonalSettings({
  profile,
  library,
}: {
  profile: PersonalProfile;
  library: PersonalLibrary;
}) {
  const t = useT();
  const [form, setForm] = useState<PersonalProfileInput>({
    start: profile.start,
    wake: profile.wake,
    sleep: profile.sleep,
    phase: profile.phase,
    baseline: profile.baseline,
    stepGoal: profile.stepGoal,
    runLevel: profile.runLevel,
  });
  const save = useSavePersonalProfile();
  const upload = useImportPersonalBackup();
  const fileInput = useRef<HTMLInputElement>(null);
  const [downloading, setDownloading] = useState(false);
  const set = (key: keyof PersonalProfileInput, value: string | number) =>
    setForm((p) => ({ ...p, [key]: value }));
  const submit = () =>
    save.mutate(form, { onSuccess: () => toast(t("Saved")) });
  const download = async () => {
    setDownloading(true);
    try {
      const data = await exportPersonalBackup();
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = `个人计划-${dateKey()}.json`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (err) {
      toast({ message: errorMessage(err), tone: "error" });
    } finally {
      setDownloading(false);
    }
  };
  const read = async (file: File) => {
    try {
      if (file.size > 2 * 1024 * 1024)
        throw new Error(t("The backup must be under 2 MB"));
      const data: PersonalBackup = JSON.parse(await file.text());
      if (
        data.version !== 1 ||
        !data.profile ||
        !data.logs ||
        !data.checks ||
        !data.sets
      )
        throw new Error(t("Choose a personal plan JSON backup"));
      const yes = await confirmAction({
        title: t("Import personal records"),
        description: t(
          "Matching fields on the same date will use the imported values. Other records will be kept.",
        ),
        confirmLabel: t("Import"),
        danger: false,
      });
      if (yes)
        upload.mutate(data, {
          onSuccess: (p) => {
            setForm({
              start: p.start,
              wake: p.wake,
              sleep: p.sleep,
              phase: p.phase,
              baseline: p.baseline,
              stepGoal: p.stepGoal,
              runLevel: p.runLevel,
            });
            toast(t("Personal backup imported"));
          },
        });
    } catch (err) {
      toast({ message: errorMessage(err), tone: "error" });
    }
  };
  return (
    <>
      <PageActions>
        <button
          className="xc-btn primary"
          title={t("Save plan settings")}
          disabled={save.isPending}
          onClick={submit}
        >
          <Save size={15} /> {t("Save plan settings")}
        </button>
      </PageActions>
      <div className="habits-personal-grid">
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Plan settings")}</h2>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <div className="habits-personal-form">
              <label className="xc-field">
                <span>{t("Plan start date")}</span>
                <input
                  className="xc-input"
                  type="date"
                  value={form.start}
                  onChange={(e) => set("start", e.target.value)}
                  required
                />
              </label>
              <label className="xc-field">
                <span>{t("Baseline weight (kg)")}</span>
                <input
                  className="xc-input"
                  type="number"
                  min="30"
                  max="250"
                  step="0.1"
                  value={form.baseline}
                  onChange={(e) => set("baseline", Number(e.target.value))}
                  required
                />
              </label>
              <label className="xc-field">
                <span>{t("Usual wake time")}</span>
                <input
                  className="xc-input"
                  type="time"
                  value={form.wake}
                  onChange={(e) => set("wake", e.target.value)}
                  required
                />
              </label>
              <label className="xc-field">
                <span>{t("Planned sleep time")}</span>
                <input
                  className="xc-input"
                  type="time"
                  value={form.sleep}
                  onChange={(e) => set("sleep", e.target.value)}
                  required
                />
              </label>
              <label className="xc-field">
                <span>{t("Strength phase")}</span>
                <select
                  className="xc-select"
                  value={form.phase}
                  onChange={(e) => set("phase", Number(e.target.value))}
                >
                  {library.phases.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} · {p.description}
                    </option>
                  ))}
                </select>
              </label>
              <label className="xc-field">
                <span>{t("Daily step reference")}</span>
                <input
                  className="xc-input"
                  type="number"
                  min="500"
                  max="30000"
                  step="500"
                  value={form.stepGoal}
                  onChange={(e) => set("stepGoal", Number(e.target.value))}
                  required
                />
              </label>
              <label className="xc-field">
                <span>{t("Walk-run level")}</span>
                <select
                  className="xc-select"
                  value={form.runLevel}
                  onChange={(e) => set("runLevel", Number(e.target.value))}
                >
                  {library.runLevels.map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.name}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <p className="xc-muted">
              {t(
                "Changing the phase keeps past records. Choose progress manually based on your recovery.",
              )}
            </p>
            <p className="xc-muted">
              {t(
                "Plan times are references. Use Daily schedule to set the shared reminder schedule.",
              )}
            </p>
            <button
              className="habits-submit-hidden"
              type="submit"
              tabIndex={-1}
              aria-hidden="true"
            />
          </form>
        </section>
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Personal plan backup")}</h2>
          </div>
          <p>
            {t(
              "Settings and daily records are saved on your server and are available across devices.",
            )}
          </p>
          <p>
            {t(
              "To move old browser records, export the JSON backup from the original HTML settings and import it here.",
            )}
          </p>
          <p className="xc-muted">
            {t(
              "The backup includes measurements, check-ins, completed sets, food notes and English notes.",
            )}
          </p>
          <div className="xc-row">
            <button
              className="xc-btn"
              disabled={downloading}
              onClick={download}
            >
              <Download size={15} /> {t("Export records")}
            </button>
            <button
              className="xc-btn"
              disabled={upload.isPending}
              onClick={() => fileInput.current?.click()}
            >
              <Upload size={15} /> {t("Import records")}
            </button>
          </div>
          <input
            type="file"
            ref={fileInput}
            accept="application/json,.json"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void read(f);
              e.target.value = "";
            }}
          />
        </section>
        <BodySync />
      </div>
    </>
  );
}
