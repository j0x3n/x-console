import { useState } from "react";
import { Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useClearScreenData,
  useCreateScreenRule,
  useDeleteScreenRule,
  useSaveScreenSettings,
  useScreenRules,
  useScreenSettings,
  type ScreenCategory,
  type ScreenSettings,
} from "./api";
import { CATEGORIES, CATEGORY_LABELS } from "./format";
import "./i18n";
import "./screentime.css";

const fail = (e: unknown) => toast({ message: errorMessage(e), tone: "error" });

/** 时间去向的设置（B116）：开关、保存标题、每台电脑、分类规则、清空。 */
export default function SettingsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useScreenSettings(open);
  const rules = useScreenRules(open);
  const save = useSaveScreenSettings();
  const create = useCreateScreenRule();
  const remove = useDeleteScreenRule();
  const clear = useClearScreenData();
  const [field, setField] = useState<"app" | "title">("app");
  const [pattern, setPattern] = useState("");
  const [category, setCategory] = useState<ScreenCategory>("coding");

  const s = settings.data;
  const update = (
    patch: Partial<Omit<ScreenSettings, "hosts">>,
    off?: string[],
  ) => {
    if (!s) return;
    const disabled = off ?? s.hosts.filter((h) => !h.enabled).map((h) => h.id);
    save.mutate(
      {
        enabled: patch.enabled ?? s.enabled,
        keepTitles: patch.keepTitles ?? s.keepTitles,
        disabledHosts: disabled,
      },
      { onError: fail },
    );
  };
  const toggleHost = (id: string, enabled: boolean) => {
    if (!s) return;
    const off = s.hosts
      .filter((h) => (h.id === id ? !enabled : !h.enabled))
      .map((h) => h.id);
    update({}, off);
  };

  const addRule = () => {
    const text = pattern.trim();
    if (!text) return;
    create.mutate(
      { field, pattern: text, category },
      {
        onSuccess: () => {
          setPattern("");
          toast(t("Rule added"));
        },
        onError: fail,
      },
    );
  };

  const onClear = async () => {
    const ok = await confirmAction({
      title: t("Clear all screen time records?"),
      description: t(
        "All records on this server are deleted. Your rules and settings stay.",
      ),
      confirmLabel: t("Clear records"),
    });
    if (!ok) return;
    clear.mutate(undefined, {
      onSuccess: () => toast(t("Records cleared")),
      onError: fail,
    });
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      wide
      title={t("Screen time settings")}
      footer={
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      }
    >
      <div className="screentime-settings">
        <div>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={s?.enabled ?? true}
              disabled={!s || save.isPending}
              onChange={(e) => update({ enabled: e.target.checked })}
            />
            <span>{t("Record which program is in front")}</span>
          </label>
          <small className="xc-check-hint">
            {t(
              "One record per minute while you are at the computer. Locked screens and more than 5 minutes without keyboard or mouse are not counted.",
            )}
          </small>
        </div>
        <div>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={s?.keepTitles ?? false}
              disabled={!s || save.isPending}
              onChange={(e) => update({ keepTitles: e.target.checked })}
            />
            <span>{t("Keep window titles")}</span>
          </label>
          <small className="xc-check-hint">
            {t(
              "Off by default. Titles often hold private text such as mail subjects. When off, the title is used to sort the minute and then dropped. Kept titles are deleted after 30 days.",
            )}
          </small>
        </div>

        <h3>{t("Computers")}</h3>
        {s && s.hosts.length === 0 ? (
          <p className="xc-muted">{t("No Windows agent is connected.")}</p>
        ) : (
          <div className="screentime-hosts">
            {s?.hosts.map((h) => (
              <div key={h.id} className="screentime-host">
                <span>
                  <strong>{h.name}</strong>
                  <span className={`xc-badge ${h.online ? "ok" : ""}`}>
                    {h.online ? t("Online") : t("Offline")}
                  </span>
                </span>
                <Switch
                  checked={h.enabled}
                  disabled={save.isPending}
                  label={h.name}
                  onChange={(v) => toggleHost(h.id, v)}
                />
              </div>
            ))}
          </div>
        )}

        <h3>{t("Sorting rules")}</h3>
        <p className="xc-muted">
          {t(
            "Your own rules come first, in the order you added them. Then the built-in rules.",
          )}{" "}
          {t("Title rules only apply to minutes whose title was kept.")}
        </p>
        {rules.data && rules.data.length === 0 && (
          <p className="xc-muted">{t("No rules yet")}</p>
        )}
        {rules.data && rules.data.length > 0 && (
          <div className="screentime-rules">
            {rules.data.map((r) => (
              <div key={r.id} className="screentime-rule">
                <span className="screentime-rule-main">
                  <small>
                    {r.field === "app"
                      ? t("Program name is")
                      : t("Title contains")}
                  </small>
                  <strong>{r.pattern}</strong>
                </span>
                <span className="xc-badge accent">
                  {t(CATEGORY_LABELS[r.category])}
                </span>
                <button
                  className="xc-btn small ghost icon"
                  aria-label={`${t("Delete rule")} ${r.pattern}`}
                  title={t("Delete rule")}
                  disabled={remove.isPending}
                  onClick={() =>
                    remove.mutate(r.id, {
                      onSuccess: () => toast(t("Rule deleted")),
                      onError: fail,
                    })
                  }
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
        <form
          className="screentime-rule-form"
          onSubmit={(e) => {
            e.preventDefault();
            addRule();
          }}
        >
          <select
            className="xc-select"
            aria-label={t("Sorting rules")}
            value={field}
            onChange={(e) => setField(e.target.value as "app" | "title")}
          >
            <option value="app">{t("Program name is")}</option>
            <option value="title">{t("Title contains")}</option>
          </select>
          <input
            className="xc-input"
            aria-label={t("Text to match")}
            placeholder={field === "app" ? "Code.exe" : t("Text to match")}
            maxLength={100}
            value={pattern}
            onChange={(e) => setPattern(e.target.value)}
          />
          <select
            className="xc-select"
            aria-label={t("Where the time went")}
            value={category}
            onChange={(e) => setCategory(e.target.value as ScreenCategory)}
          >
            {CATEGORIES.map((c) => (
              <option key={c} value={c}>
                {t(CATEGORY_LABELS[c])}
              </option>
            ))}
          </select>
          <button
            type="submit"
            className="xc-btn"
            disabled={!pattern.trim() || create.isPending}
          >
            {t("Add rule")}
          </button>
        </form>

        <div className="screentime-danger">
          <button
            className="xc-btn danger"
            disabled={clear.isPending}
            onClick={onClear}
          >
            <Trash2 size={14} /> {t("Clear all records")}
          </button>
        </div>
      </div>
    </Dialog>
  );
}
