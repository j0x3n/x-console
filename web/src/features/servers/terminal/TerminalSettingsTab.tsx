import { useT } from "../../../contexts/LanguageContext";
import {
  usePreferencesStore,
  type TerminalLeave,
} from "../../../stores/preferences-store";
import "./i18n";

const options: { value: TerminalLeave; label: string }[] = [
  { value: "ask", label: "Ask every time" },
  { value: "keep", label: "Keep in background" },
  { value: "close", label: "Disconnect it" },
];

/** B249：设置 → 通用 → 终端。离开弹窗里勾了“不再问”以后，在这里改回来。 */
export default function TerminalSettingsTab() {
  const t = useT();
  const leave = usePreferencesStore((s) => s.terminalLeave);
  const setLeave = usePreferencesStore((s) => s.setTerminalLeave);
  return (
    <div className="settings-grid">
      <div className="xc-card">
        <div className="settings-line">
          <div>
            <label htmlFor="settings-terminal-leave">
              <strong>{t("When leaving a connected terminal")}</strong>
            </label>
            <small>
              {t(
                "Applies when you leave the server or computer page while a terminal is connected.",
              )}
            </small>
          </div>
          <select
            id="settings-terminal-leave"
            className="xc-select"
            value={leave}
            onChange={(e) => setLeave(e.target.value as TerminalLeave)}
          >
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.label)}
              </option>
            ))}
          </select>
        </div>
      </div>
    </div>
  );
}
