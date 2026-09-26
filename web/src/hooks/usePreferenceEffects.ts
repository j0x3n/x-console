import { useEffect, useState } from "react";
import { usePreferencesStore } from "../stores/preferences-store";

/** 把主题和语言偏好同步到 <html>。在根组件调用一次。 */
export function usePreferenceEffects() {
  const language = usePreferencesStore((s) => s.language);
  const themeMode = usePreferencesStore((s) => s.themeMode);
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
  useEffect(() => {
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  }, [language]);
  return language;
}
