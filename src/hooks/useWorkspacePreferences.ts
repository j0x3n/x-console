import { useState, useEffect } from "react";
import { translate } from "../lib/i18n";
import { usePreferencesStore } from "../stores/preferences-store";
import type { Text } from "../types/domain";

export function useWorkspacePreferences(view: string) {
  const language = usePreferencesStore((state) => state.language);
  const themeMode = usePreferencesStore((state) => state.themeMode);
  const setLanguage = usePreferencesStore((state) => state.setLanguage);
  const setThemeMode = usePreferencesStore((state) => state.setThemeMode);
  const [systemDark, setSystemDark] = useState(
    () => window.matchMedia("(prefers-color-scheme: dark)").matches,
  );
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const update = (event: MediaQueryListEvent) => setSystemDark(event.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  useEffect(() => {
    const resolved =
      themeMode === "system" ? (systemDark ? "dark" : "light") : themeMode;
    document.documentElement.dataset.theme = resolved;
    document.documentElement.style.colorScheme = resolved;
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", resolved === "dark" ? "#0a0a0b" : "#f3f3f4");
  }, [themeMode, systemDark]);
  const t = (text: Text) => translate(language, text);
  useEffect(() => {
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
    document.title = `${t(view)} · X Console`;
  }, [language, view]);

  return { language, setLanguage, themeMode, setThemeMode, t };
}
