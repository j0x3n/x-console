import { useSyncExternalStore } from "react";
import type { LucideIcon } from "lucide-react";
import type { NavigateFunction } from "react-router";

/*
 * 命令面板（⌘K）的命令注册表。模块在自己的 routes.tsx 里调用
 * registerCommands([...])，例如“新建 Issue”“开始番茄钟”。
 */
export interface Command {
  id: string;
  title: string;
  group: string;
  keywords?: string;
  icon?: LucideIcon;
  /**
   * 可选的前缀，比如 ">"。输入框里的内容以它开头时，面板只显示这个命令，
   * 回车后把前缀后面的文字作为 text 传给 run。
   */
  prefix?: string;
  run: (ctx: {
    navigate: NavigateFunction;
    text?: string;
  }) => void | Promise<void>;
}

/** 输入内容匹配到带前缀的命令时，返回命令和前缀后面的文字。 */
export function matchPrefix(
  query: string,
  list: Command[],
): { command: Command; text: string } | null {
  const q = query.trimStart();
  // 前缀长的先比，免得 ">>" 被 ">" 抢走。
  const withPrefix = list
    .filter((c) => c.prefix)
    .sort((a, b) => b.prefix!.length - a.prefix!.length);
  for (const command of withPrefix) {
    if (q.startsWith(command.prefix!))
      return { command, text: q.slice(command.prefix!.length).trim() };
  }
  return null;
}

let commands: Command[] = [];
const subscribers = new Set<() => void>();

export function registerCommands(list: Command[]) {
  const ids = new Set(list.map((c) => c.id));
  commands = [...commands.filter((c) => !ids.has(c.id)), ...list];
  subscribers.forEach((fn) => fn());
}

export function useCommands(): Command[] {
  return useSyncExternalStore(
    (fn) => {
      subscribers.add(fn);
      return () => subscribers.delete(fn);
    },
    () => commands,
  );
}
