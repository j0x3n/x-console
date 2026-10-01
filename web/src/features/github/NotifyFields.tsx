import { useT } from "../../contexts/LanguageContext";
import type { RepoNotify } from "./api";
import { NOTIFY_GROUPS, NOTIFY_LABELS } from "./logic";

/** B71：哪些仓库事件发通知。设置页的默认设置和单个仓库的弹窗共用。 */
export default function NotifyFields({
  value,
  onChange,
  disabled,
}: {
  value: RepoNotify;
  onChange: (next: RepoNotify) => void;
  disabled?: boolean;
}) {
  const t = useT();
  const on = new Set(value.events);
  const toggle = (event: string, checked: boolean) => {
    const next = new Set(on);
    if (checked) next.add(event);
    else next.delete(event);
    onChange({ ...value, events: [...next] });
  };
  return (
    <div className="repos-notify">
      {NOTIFY_GROUPS.map((g) => (
        <fieldset key={g.label} className="repos-notify-group">
          <legend>{t(g.label)}</legend>
          {g.events.map((e) => (
            <label key={e} className="xc-check">
              <input
                type="checkbox"
                checked={on.has(e)}
                disabled={disabled}
                onChange={(ev) => toggle(e, ev.target.checked)}
              />
              <span>{t(NOTIFY_LABELS[e] ?? e)}</span>
            </label>
          ))}
        </fieldset>
      ))}
      <label className="xc-field repos-notify-branches">
        <span>{t("CI events on")}</span>
        <select
          className="xc-select"
          value={value.ciBranches}
          disabled={disabled}
          onChange={(e) =>
            onChange({
              ...value,
              ciBranches: e.target.value as RepoNotify["ciBranches"],
            })
          }
        >
          <option value="default">{t("Default branch only")}</option>
          <option value="all">{t("All branches and pull requests")}</option>
        </select>
      </label>
    </div>
  );
}
