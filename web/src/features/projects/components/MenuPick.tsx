import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Check } from "lucide-react";


export interface MenuPickOption<T extends string | number> {
  value: T;
  label: string;
  icon?: ReactNode;
  group?: string;
}

/** 抽屉里的选项：按钮看着像普通文字，点开是自己的菜单，不用系统下拉框。 */
export default function MenuPick<T extends string | number>({
  label,
  value,
  options,
  empty,
  onChange,
}: {
  label: string;
  value: T;
  options: MenuPickOption<T>[];
  empty?: boolean;
  onChange: (value: T) => void;
}) {
  const current = options.find((o) => o.value === value) ?? options[0];
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<CSSProperties>({
    visibility: "hidden",
  });
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const place = () => {
    const button = buttonRef.current?.getBoundingClientRect();
    const menu = menuRef.current?.getBoundingClientRect();
    if (!button || !menu) return;
    if (window.matchMedia?.("(max-width: 640px)").matches) {
      setStyle({});
      return;
    }
    const left = Math.max(
      8,
      Math.min(button.left, window.innerWidth - menu.width - 8),
    );
    const below = button.bottom + 4;
    const top =
      below + menu.height <= window.innerHeight - 8
        ? below
        : Math.max(8, button.top - 4 - menu.height);
    setStyle({ top, left });
  };

  useLayoutEffect(() => {
    if (open) place();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopImmediatePropagation();
      setOpen(false);
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [open]);

  const items: ReactNode[] = [];
  let lastGroup = "";
  for (const option of options) {
    if (option.group && option.group !== lastGroup) {
      lastGroup = option.group;
      items.push(
        <div key={`g:${option.group}`} className="projects-pill-group">
          {option.group}
        </div>,
      );
    }
    const selected = option.value === value;
    items.push(
      <button
        key={String(option.value)}
        type="button"
        role="option"
        aria-selected={selected}
        className={selected ? "on" : ""}
        onClick={() => {
          setOpen(false);
          if (!selected) onChange(option.value);
        }}
      >
        {option.icon}
        <span>{option.label}</span>
        {selected && <Check size={14} className="projects-pill-check" />}
      </button>,
    );
  }

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className={`projects-pill${empty || !current ? " empty" : ""}`}
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        {current?.icon}
        <span>{current?.label ?? label}</span>
      </button>
      {open &&
        createPortal(
          <>
            <div
              className="xc-more-backdrop"
              onClick={() => setOpen(false)}
              onContextMenu={(e) => {
                e.preventDefault();
                setOpen(false);
              }}
            />
            <div
              ref={menuRef}
              className="xc-more-menu projects-pill-menu enter"
              role="listbox"
              aria-label={label}
              style={style}
            >
              <div className="xc-more-title">{label}</div>
              {items}
            </div>
          </>,
          document.body,
        )}
    </>
  );
}
