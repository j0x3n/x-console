import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { CornerDownLeft, Search } from "lucide-react";
import { navItems } from "../../app/nav";
import { useT } from "../../contexts/LanguageContext";
import { useCommands, type Command } from "../../lib/commands";

/** ⌘K 命令面板：跳转页面和执行模块注册的命令。 */
export default function CommandPalette({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const navigate = useNavigate();
  const registered = useCommands();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const all = useMemo<Command[]>(
    () => [
      ...navItems.map((item) => ({
        id: `nav:${item.path}`,
        title: t(item.label),
        group: t("Go to"),
        keywords: item.label,
        icon: item.icon,
        run: () => navigate(item.path),
      })),
      ...registered,
    ],
    [registered, navigate, t],
  );
  const results = useMemo(() => {
    const prefix = all.find(
      (command) => command.prefix && query.startsWith(command.prefix),
    );
    if (prefix) return [prefix];
    const q = query.trim().toLowerCase();
    if (!q) return all.slice(0, 30);
    return all
      .filter((c) =>
        `${c.title} ${c.keywords ?? ""} ${c.group}`.toLowerCase().includes(q),
      )
      .slice(0, 30);
  }, [all, query]);
  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
    }
  }, [open]);
  useEffect(() => setActive(0), [query]);
  if (!open) return null;
  const run = async (command?: Command) => {
    if (!command) return;
    const input =
      command.prefix && query.startsWith(command.prefix)
        ? query.slice(command.prefix.length).trim()
        : undefined;
    if (command.prefix && !input) return;
    onClose();
    await command.run({ navigate, input });
  };
  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        className="command-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={t("Search")}
      >
        <div className="command-input">
          <Search size={17} />
          <input
            autoFocus
            value={query}
            placeholder={t("Search or run a command...")}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((i) => Math.min(i + 1, results.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((i) => Math.max(i - 1, 0));
              } else if (e.key === "Enter") {
                e.preventDefault();
                run(results[active]);
              } else if (e.key === "Escape") onClose();
            }}
          />
          <kbd>esc</kbd>
        </div>
        <div className="command-results" role="listbox">
          {results.map((command, index) => (
            <button
              key={command.id}
              role="option"
              aria-selected={index === active}
              className={index === active ? "active" : ""}
              onMouseEnter={() => setActive(index)}
              onClick={() => run(command)}
            >
              {command.icon && <command.icon size={15} />}
              <span>
                {command.prefix && query.startsWith(command.prefix)
                  ? `${command.title}：${query.slice(command.prefix.length).trim() || "输入内容"}`
                  : command.title}
              </span>
              <small className="command-caption">{command.group}</small>
              {index === active && <CornerDownLeft size={13} />}
            </button>
          ))}
          {results.length === 0 && (
            <div className="empty-result">{t("No results found")}</div>
          )}
        </div>
      </div>
    </div>
  );
}
