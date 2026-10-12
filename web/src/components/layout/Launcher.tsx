import { useEffect, useRef, useState } from "react";
import { Pin, Search, X } from "lucide-react";
import { launcherGroupLabels, type NavGroup } from "../../app/nav";
import { useT } from "../../contexts/LanguageContext";
import type { NavEntry } from "./navEntries";

/**
 * 启动台（B179）：全部入口按分组排成图标网格。点入口跳过去，点图钉固定到左栏或取消。
 * 顶部搜索按名字过滤，回车打开第一个。Esc 或点外面关闭。手机上占满屏幕。
 */
export default function Launcher({
  entries,
  pinned,
  onOpen,
  onTogglePin,
  onClose,
}: {
  entries: NavEntry[];
  pinned: (path: string) => boolean;
  onOpen: (entry: NavEntry) => void;
  onTogglePin: (path: string) => void;
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
