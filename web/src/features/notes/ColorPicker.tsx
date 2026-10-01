import { Ban, Check } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import type { NoteColor } from "./api";
import { NOTE_COLORS, noteBgClass } from "./noteColors";

/** 一排颜色圆点（B74），笔记的“更多”菜单和便签卡片都用它。 */
export default function ColorPicker({
  value,
  onChange,
}: {
  value?: string;
  onChange: (color: NoteColor) => void;
}) {
  const t = useT();
  return (
    <div
      className="notes-colors"
      role="radiogroup"
      aria-label={t("Background")}
    >
      {NOTE_COLORS.map((c) => {
        const on = (value ?? "") === c.id;
        return (
          <button
            key={c.id || "default"}
            type="button"
            role="radio"
            aria-checked={on}
            aria-label={t(c.label)}
            title={t(c.label)}
            className={`notes-color ${c.id ? noteBgClass(c.id) : "none"}${on ? " on" : ""}`}
            onClick={(e) => {
              e.stopPropagation();
              onChange(c.id);
            }}
          >
            {on ? <Check size={12} /> : !c.id ? <Ban size={12} /> : null}
          </button>
        );
      })}
    </div>
  );
}
