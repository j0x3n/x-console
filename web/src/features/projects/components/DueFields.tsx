import { useT } from "../../../contexts/LanguageContext";
import {
  DUE_REMINDS,
  DUE_REMIND_LABELS,
  localDate,
  type DueRemind,
} from "../logic";
import MenuPick from "./MenuPick";

export interface DueValue {
  date: string;
  /** 空表示没选时间，保存时按 23:59 */
  time: string;
}

/**
 * 截止时间：日期加时间，可以只选日期。live 为 false 时后端还不支持到分钟，只显示日期。
 * 传了 onRemindChange 才显示“提前提醒”。
 */
export default function DueFields({
  value,
  onChange,
  live,
  remind,
  onRemindChange,
  menu,
}: {
  value: DueValue;
  onChange: (value: DueValue) => void;
  live: boolean;
  remind?: DueRemind;
  onRemindChange?: (remind: DueRemind) => void;
  menu?: boolean;
}) {
  const t = useT();
  return (
    <div className="projects-due-fields">
      <input
        className="xc-input"
        type="date"
        value={value.date}
        aria-label={t("Due date")}
        onChange={(e) => onChange({ ...value, date: e.target.value })}
      />
      {live && (
        <input
          className="xc-input projects-time-input"
          type="time"
          value={value.time}
          aria-label={t("Due time")}
          onChange={(e) =>
            // 先选时间时，日期默认今天。
            onChange({
              date: value.date || (e.target.value ? localDate() : ""),
              time: e.target.value,
            })
          }
        />
      )}
      {live && onRemindChange && value.date &&
        (menu ? (
          <MenuPick
            label={t("Remind ahead")}
            value={remind ?? "at_due"}
            options={DUE_REMINDS.map((r) => ({
              value: r,
              label: t(DUE_REMIND_LABELS[r]),
            }))}
            onChange={onRemindChange}
          />
        ) : (
          <select
            className="xc-select"
            value={remind ?? "at_due"}
            aria-label={t("Remind ahead")}
            onChange={(e) => onRemindChange(e.target.value as DueRemind)}
          >
            {DUE_REMINDS.map((r) => (
              <option key={r} value={r}>
                {t(DUE_REMIND_LABELS[r])}
              </option>
            ))}
          </select>
        ))}
    </div>
  );
}
