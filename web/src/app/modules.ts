import { useQuery } from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../api/client";
import type { components, paths } from "../api/gen/vault";

/*
 * 这个会话能用哪些模块（B57）。隐藏内容锁着时，服务端不返回被隐藏的模块，
 * 左栏、路由、今日页、命令面板都按它过滤，看起来就像没有这个模块。
 * 接口没上线（404、501）时全部能用。
 */

export type ModuleId = components["schemas"]["ModuleId"];

const api = createApi<paths>();

/** 所有能隐藏的模块，顺序和左栏一致。今日页不能隐藏。 */
export const ALL_MODULES: ModuleId[] = [
  "projects",
  "coding",
  "notes",
  "mail",
  "reminders",
  "habits",
  "drive",
  "drive-webdav",
  "drive-gdrive",
  "calendar",
  "servers",
  "pc",
  "monitoring",
  "home",
  "router",
  "automations",
  "github",
  "documents",
  "screentime",
  "readlater",
];

/** 不在 vault 前缀下：锁定时 vault 会重置别的缓存，这里要跟着重新拉。 */
export const moduleKeys = { available: ["app", "modules"] as const };

/** 地址属于哪个模块。今日页、设置这类不属于可隐藏模块的返回 null。 */
export function moduleOfPath(pathname: string): ModuleId | null {
  // 早报的地址在 /calendar 下面，但属于今日页，不跟着日程隐藏（B66）
  if (
    pathname === "/calendar/briefs" ||
    pathname.startsWith("/calendar/briefs/")
  )
    return null;
  const first = pathname.split("/")[1] ?? "";
  return (ALL_MODULES as string[]).includes(first) ? (first as ModuleId) : null;
}

/** 命令面板的分组属于哪个模块。 */
const commandGroups: Record<string, ModuleId> = {
  项目: "projects",
  Issue: "projects",
  "Agent 任务": "coding",
  笔记: "notes",
  邮件: "mail",
  提醒: "reminders",
  习惯: "habits",
  云盘: "drive",
  日历: "calendar",
  服务器: "servers",
  本机: "pc",
  监控: "monitoring",
  智能家居: "home",
  路由器: "router",
  自动化: "automations",
  GitHub: "github",
  证件档案: "documents",
  电脑时间: "screentime",
  稍后读: "readlater",
};

export function moduleOfCommandGroup(group: string): ModuleId | null {
  return commandGroups[group] ?? null;
}

export interface ModuleAccess {
  /** 还在加载，这时可隐藏的模块先不显示，避免闪一下 */
  pending: boolean;
  /** 这个模块能不能用。不属于任何模块（今日页、设置）的总是 true */
  has: (id: ModuleId | null) => boolean;
}

export function useModules(): ModuleAccess {
  const q = useQuery({
    queryKey: moduleKeys.available,
    queryFn: async (): Promise<ModuleId[]> => {
      try {
        const out = await unwrap(api.GET("/app/modules"));
        // 测试里的假接口或者旧服务端可能不给这个字段，这时全部能用
        return Array.isArray(out?.modules) ? out.modules : ALL_MODULES;
      } catch (error) {
        if (
          error instanceof ApiError &&
          (error.status === 404 || error.status === 501)
        )
          return ALL_MODULES;
        throw error;
      }
    },
    staleTime: 60_000,
    meta: { silentError: true },
  });
  // 出错时全部显示，不能因为一个接口坏了把左栏清空
  const list = q.isError ? ALL_MODULES : q.data;
  return {
    pending: !list,
    has: (id) => id === null || (list?.includes(id) ?? false),
  };
}
