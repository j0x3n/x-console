import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { coreApi } from "../api/core";
import { unwrap } from "../api/client";
import {
  usePreferencesStore,
  type PreferencesState,
} from "../stores/preferences-store";
import type { components } from "../api/gen/core";
import type { ThemeMode } from "../types/domain";

type Preferences = components["schemas"]["Preferences"];

const toNight: Record<ThemeMode, Preferences["nightMode"]> = {
  dark: "on",
  light: "off",
  system: "auto",
};
const fromNight: Record<Preferences["nightMode"], ThemeMode> = {
  on: "dark",
  off: "light",
  auto: "system",
};

let applying = false;

/*
 * 夜间模式、主题色、语言存在服务端（B22），换一台浏览器登录也一样。
 * 本地 localStorage 也存一份，首屏不闪。接口还没上线时只用本地的。
 * 在登录后的布局里调用一次。
 */
export function usePreferencesSync() {
  const remote = useQuery({
    queryKey: ["me", "preferences"],
    queryFn: () => unwrap(coreApi.GET("/me/preferences")),
    retry: false,
    staleTime: Infinity,
  });
  useEffect(() => {
    const p = remote.data;
    if (!p) return;
    // 服务端从没存过：把这台浏览器的偏好存上去，不用默认值覆盖本地。
    if (!p.updatedAt) {
      save(usePreferencesStore.getState());
      return;
    }
    applying = true;
    const store = usePreferencesStore.getState();
    store.setThemeMode(fromNight[p.nightMode] ?? "system");
    store.setAccent(p.accent);
    store.setLanguage(p.language);
    applying = false;
  }, [remote.data]);
  const live = remote.isSuccess;
  useEffect(() => {
    if (!live) return;
    return usePreferencesStore.subscribe((s, prev) => {
      if (applying) return;
      if (
        s.themeMode === prev.themeMode &&
        s.accent === prev.accent &&
        s.language === prev.language
      )
        return;
      save(s);
    });
  }, [live]);
}

function save(s: Pick<PreferencesState, "themeMode" | "accent" | "language">) {
  coreApi
    .PUT("/me/preferences", {
      body: {
        nightMode: toNight[s.themeMode],
        accent: s.accent,
        language: s.language,
      },
    })
    .catch(() => {
      /* 存不上服务端时本地仍然生效 */
    });
}
