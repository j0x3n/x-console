import Switch from "../../../components/ui/Switch";
import { useT } from "../../../contexts/LanguageContext";
import { useNoteAiSettings, useSaveNoteAiSettings } from "../../notes/api";

/** 笔记自动起标题、自动加标签（B32）。没配 AI 模型时不显示。 */
export default function NotesAiCard() {
  const t = useT();
  const settings = useNoteAiSettings();
  const save = useSaveNoteAiSettings();
  const s = settings.data;
  if (!s || !s.available) return null;
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Notes")}</h2>
      </div>
      <div className="ai-switch-row">
        <span>
          <strong>{t("Title notes automatically")}</strong>
          <small>
            {t(
              "Fills in an empty title 3 seconds after saving. Hidden notes are never sent.",
            )}
          </small>
        </span>
        <Switch
          checked={s.autoTitle}
          label={t("Title notes automatically")}
          onChange={(autoTitle) => save.mutate({ autoTitle })}
        />
      </div>
      <div className="ai-switch-row">
        <span>
          <strong>{t("Tag notes automatically")}</strong>
          <small>{t("Picks up to 3 of your existing tags.")}</small>
        </span>
        <Switch
          checked={s.autoTags}
          label={t("Tag notes automatically")}
          onChange={(autoTags) => save.mutate({ autoTags })}
        />
      </div>
      {s.autoTags && (
        <label className="xc-field">
          <span>{t("Suggested tags")}</span>
          <select
            className="xc-select"
            value={s.tagMode}
            onChange={(e) =>
              save.mutate({ tagMode: e.target.value as "suggest" | "apply" })
            }
          >
            <option value="suggest">{t("Show as suggestions first")}</option>
            <option value="apply">{t("Add them directly")}</option>
          </select>
        </label>
      )}
    </section>
  );
}
