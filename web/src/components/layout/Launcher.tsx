import { useEffect, useRef, useState } from "react";
import { GripVertical, Minus, Pin, Search, X } from "lucide-react";
import { launcherGroupLabels, type NavGroup } from "../../app/nav";
import { useT } from "../../contexts/LanguageContext";
import {
  isNavSeparator,
  movePin,
  newSeparator,
  type NavEntry,
} from "./navEntries";

/**
 * 启动台（B179）：全部入口按分组排成图标网格。点入口跳过去，点图钉固定到左栏或取消。
 * 顶部搜索按名字过滤，回车打开第一个。Esc 或点外面关闭。手机上占满屏幕。
 */
export default function Launcher({
  entries,
  pins,
  pinned,
  onOpen,
  onTogglePin,
  onChangePins,
  onClose,
}: {
  entries: NavEntry[];
  pins?: string[];
  pinned: (path: string) => boolean;
  onOpen: (entry: NavEntry) => void;
  onTogglePin: (path: string) => void;
  onChangePins?: (pins: string[]) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [query, setQuery] = useState("");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    input.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);
  const q = query.trim().toLowerCase();
  const names = (e: NavEntry) => [e.label, ...e.members.map((m) => m.label)];
  const matches = q
    ? entries.filter((e) =>
        names(e).some(
          (n) => t(n).toLowerCase().includes(q) || n.toLowerCase().includes(q),
        ),
      )
    : entries;
  const groups = (Object.keys(launcherGroupLabels) as NavGroup[])
    .map((group) => ({
      group,
      items: matches.filter((e) => e.group === group),
    }))
    .filter((g) => g.items.length > 0);
  return (
    <div
      className="modal-backdrop launcher-backdrop"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <div
        className="launcher"
        role="dialog"
        aria-modal="true"
        aria-label={t("All apps")}
      >
        <div className="launcher-head">
          <form
            className="launcher-search"
            role="search"
            onSubmit={(event) => {
              event.preventDefault();
              if (matches[0]) onOpen(matches[0]);
            }}
          >
            <Search size={15} />
            <input
              ref={input}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("Search apps")}
              aria-label={t("Search apps")}
            />
          </form>
          <button
            type="button"
            className="icon-button"
            aria-label={t("Close")}
            title={t("Close")}
            onClick={onClose}
          >
            <X size={16} />
          </button>
        </div>
        <div className="launcher-body">
          {!q && pins && onChangePins && (
            <RailOrder entries={entries} pins={pins} onChange={onChangePins} />
          )}
          {groups.length === 0 && (
            <div className="launcher-empty">{t("No matching apps")}</div>
          )}
          {groups.map(({ group, items }) => (
            <section key={group} className="launcher-group">
              <h3>{t(launcherGroupLabels[group])}</h3>
              <div className="launcher-grid">
                {items.map((entry) => {
                  const on = pinned(entry.path);
                  const Icon = entry.icon;
                  const pinLabel = on
                    ? t("Unpin from sidebar")
                    : t("Pin to sidebar");
                  return (
                    <div
                      key={entry.path}
                      className={`launcher-tile${on ? " pinned" : ""}`}
                    >
                      <button
                        type="button"
                        className="launcher-open"
                        onClick={() => onOpen(entry)}
                      >
                        <span className="launcher-icon">
                          <Icon size={20} strokeWidth={1.6} />
                        </span>
                        <span className="launcher-name">{t(entry.label)}</span>
                        {entry.members.length > 1 && (
                          <small>
                            {entry.members.map((m) => t(m.label)).join(" · ")}
                          </small>
                        )}
                      </button>
                      <button
                        type="button"
                        className="launcher-pin"
                        aria-label={`${pinLabel}: ${t(entry.label)}`}
                        aria-pressed={on}
                        title={pinLabel}
                        onClick={() => onTogglePin(entry.path)}
                      >
                        <Pin size={13} />
                      </button>
                    </div>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * 左栏顺序（B189）：固定的入口和分隔线按顺序排一行，拖动或用左右方向键调整，
 * 点叉取消固定或删掉分隔线。被隐藏的模块不显示，但在列表里的位置不变。
 */
function RailOrder({
  entries,
  pins,
  onChange,
}: {
  entries: NavEntry[];
  pins: string[];
  onChange: (pins: string[]) => void;
}) {
  const t = useT();
  const [dragging, setDragging] = useState<number | null>(null);
  const items = pins
    .map((pin, index) => ({
      pin,
      index,
      entry: entries.find((e) => e.path === pin),
    }))
    .filter((item) => item.entry || isNavSeparator(item.pin));
  const move = (from: number, to: number) => {
    if (from !== to) onChange(movePin(pins, from, to));
  };
  return (
    <section className="launcher-group launcher-rail">
      <div className="launcher-rail-head">
        <h3>{t("Sidebar order")}</h3>
        <button
          type="button"
          className="xc-btn small ghost"
          onClick={() => onChange([...pins, newSeparator(pins)])}
        >
          <Minus size={13} />
          {t("Add divider")}
        </button>
      </div>
      <div className="launcher-rail-list" role="list">
        {items.map(({ pin, index, entry }, i) => {
          const sep = !entry;
          const Icon = entry?.icon;
          const label = entry ? t(entry.label) : t("Divider");
          return (
            <div
              key={pin}
              role="listitem"
              tabIndex={0}
              draggable
              className={`launcher-rail-chip${sep ? " sep" : ""}${dragging === index ? " dragging" : ""}`}
              title={t("Drag to reorder")}
              aria-label={label}
              onDragStart={(e) => {
                setDragging(index);
                e.dataTransfer.effectAllowed = "move";
              }}
              onDragEnd={() => setDragging(null)}
              onDragOver={(e) => {
                if (dragging === null) return;
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
              }}
              onDrop={(e) => {
                e.preventDefault();
                if (dragging !== null) move(dragging, index);
                setDragging(null);
              }}
              onKeyDown={(e) => {
                const target =
                  e.key === "ArrowLeft"
                    ? items[i - 1]
                    : e.key === "ArrowRight"
                      ? items[i + 1]
                      : undefined;
                if (!target) return;
                e.preventDefault();
                move(index, target.index);
              }}
            >
              <GripVertical size={12} className="launcher-rail-grip" />
              {Icon ? <Icon size={14} strokeWidth={1.6} /> : <i aria-hidden />}
              <span>{label}</span>
              <button
                type="button"
                className="launcher-rail-remove"
                aria-label={`${sep ? t("Remove divider") : t("Unpin from sidebar")}: ${label}`}
                title={sep ? t("Remove divider") : t("Unpin from sidebar")}
                onClick={() => onChange(pins.filter((p) => p !== pin))}
              >
                <X size={12} />
              </button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
