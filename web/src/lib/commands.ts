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
  run: (ctx: { navigate: NavigateFunction }) => void | Promise<void>;
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
