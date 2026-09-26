import { useT } from "../../contexts/LanguageContext";
import { usePreferencesStore } from "../../stores/preferences-store";
import type { Language, ThemeMode } from "../../types/domain";

export default function GeneralTab() {
  const t = useT();
  const { language, setLanguage, themeMode, setThemeMode } =
    usePreferencesStore();
  return (
    <div className="xc-card" style={{ maxWidth: 520 }}>
      <div className="xc-card-head">
        <h2>{t("Appearance")}</h2>
      </div>
      <label className="xc-field">
        <span>{t("Theme")}</span>
        <select
          className="xc-select"
          value={themeMode}
          onChange={(e) => setThemeMode(e.target.value as ThemeMode)}
        >
          <option value="system">{t("Follow system")}</option>
          <option value="dark">{t("Dark")}</option>
          <option value="light">{t("Light")}</option>
        </select>
      </label>
      <label className="xc-field">
        <span>{t("Language")}</span>
        <select
          className="xc-select"
          value={language}
          onChange={(e) => setLanguage(e.target.value as Language)}
        >
          <option value="zh">中文</option>
          <option value="en">English</option>
        </select>
      </label>
    </div>
  );
}
