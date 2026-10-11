import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { withElevation } from "../../auth/elevation";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/focusblock";

/* 专注屏蔽（B131）：番茄钟进行时，代理改 hosts、结束分心的程序。 */
export const focusBlockApi = createApi<paths>();

type S = components["schemas"];
export type FocusBlockSettings = S["FocusBlockSettings"];
export type FocusBlockInput = S["FocusBlockSettingsInput"];
export type FocusBlockComputer = S["FocusBlockComputer"];
export type FocusBlockStatus = S["FocusBlockStatus"];

export const focusBlockKeys = {
  all: ["focusblock"] as const,
  settings: ["focusblock", "settings"] as const,
  status: ["focusblock", "status"] as const,
};

// 屏蔽生效、结束或改了设置时刷新
invalidateOn("focusblock.", focusBlockKeys.all);

export function useFocusBlockSettings() {
  return useQuery({
    queryKey: focusBlockKeys.settings,
    queryFn: () => unwrap(focusBlockApi.GET("/focus-block/settings")),
    retry: false,
  });
}

/** 只改传了的字段。名单决定了代理会结束哪些程序，所以要先提升权限。 */
export function usePutFocusBlockSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: FocusBlockInput) =>
      withElevation(() =>
        unwrap(focusBlockApi.PUT("/focus-block/settings", { body })),
      ),
    onSuccess: () => client.invalidateQueries({ queryKey: focusBlockKeys.all }),
  });
}

/** 番茄钟页上的“屏蔽中”小标用它。接口还没上线时不报错，什么都不显示。 */
export function useFocusBlockStatus() {
  return useQuery({
    queryKey: focusBlockKeys.status,
    queryFn: () => unwrap(focusBlockApi.GET("/focus-block/status")),
    retry: false,
    meta: { silentError: true },
  });
}

/** 电脑时间里分到“娱乐”类的程序。点“导入”时才取。 */
export async function fetchSuggestions(): Promise<string[]> {
  const out = await unwrap(focusBlockApi.GET("/focus-block/suggestions"));
  return out.apps;
}
