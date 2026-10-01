import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { CornerDownLeft, Search } from "lucide-react";
import { navItems } from "../../app/nav";
import {
  moduleOfCommandGroup,
  moduleOfPath,
  useModules,
} from "../../app/modules";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { matchPrefix, useCommands, type Command } from "../../lib/commands";

/** 没输入时显示的常用命令，按这个顺序。输入后才搜全部页面和命令。 */
const SUGGESTED = [
  "notes.quick",
  "notes.new",
  "projects.new-issue",
  "reminders.new",
  "assistant.open",
  "drive.search",
  "calendar.today",
  "settings.open",
];

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
  // B57：被隐藏的模块，它的页面和命令都不出现
  const modules = useModules();
  const allRegistered = useCommands();
  const registered = useMemo(
    () =>
      allRegistered.filter((c) => modules.has(moduleOfCommandGroup(c.group))),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [allRegistered, modules.has],
  );
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const all = useMemo<Command[]>(
    () => [
      ...navItems
        .filter((item) => modules.has(moduleOfPath(item.path)))
        .map((item) => ({
          id: `nav:${item.path}`,
          title: t(item.label),
          group: t("Go to"),
          keywords: item.label,
          icon: item.icon,
          run: () => navigate(item.path),
        })),
      ...registered,
    ],
    [registered, navigate, t, modules.has],
  );
  const prefixed = useMemo(
    () => matchPrefix(query, registered),
    [query, registered],
  );
  const prefixes = registered.filter((c) => c.prefix);
  const results = useMemo(() => {
    if (prefixed) return [prefixed.command];
    const q = query.trim().toLowerCase();
    // 没输入时只列几个常用的，不把所有页面和命令都摆出来。
    if (!q) {
      const byId = new Map(registered.map((c) => [c.id, c]));
      const picked = SUGGESTED.map((id) => byId.get(id)).filter(
        (c): c is Command => !!c,
      );
      return picked.length > 0 ? picked : registered.slice(0, SUGGESTED.length);
    }
    // 标题完全一样的排第一，标题开头一样的第二，其余按原来的顺序。
    const score = (c: Command) => {
      const title = c.title.toLowerCase();
      if (title === q) return 0;
      if (title.startsWith(q)) return 1;
      if (title.includes(q)) return 2;
      return 3;
    };
    return all
      .filter((c) =>
        `${c.title} ${c.keywords ?? ""} ${c.group}`.toLowerCase().includes(q),
      )
      .map((c, i) => ({ c, i, s: score(c) }))
      .sort((a, b) => a.s - b.s || a.i - b.i)
      .map((x) => x.c)
      .slice(0, 30);
  }, [all, query, prefixed]);
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
    if (command.prefix) {
      // 带前缀的命令要有文字。还没输入时，把前缀填进输入框。
      const text = prefixed?.command.id === command.id ? prefixed.text : "";
      if (!text) {
        setQuery(`${command.prefix} `);
        return;
      }
      onClose();
      try {
        await command.run({ navigate, text });
      } catch (error) {
        toast({
          message: error instanceof Error ? error.message : String(error),
          tone: "error",
        });
      }
      return;
    }
    onClose();
    await command.run({ navigate });
  };
  return (
    <div
      className="modal-backdrop command-backdrop"
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
                {command.prefix && prefixed?.command.id === command.id
                  ? prefixed.text
                    ? `${command.title}：${prefixed.text}`
                    : `${command.title}：${t("type the text after the prefix")}`
                  : command.prefix
                    ? `${command.title}（${command.prefix} …）`
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
        {prefixes.length > 0 && !prefixed && (
          <div className="command-footer">
            {prefixes.map((c) => (
              <span key={c.id}>
                <kbd>{c.prefix}</kbd>
                {c.title}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
