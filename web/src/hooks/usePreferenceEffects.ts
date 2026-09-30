import { useEffect, useState } from "react";
import { usePreferencesStore } from "../stores/preferences-store";

/** 把主题和语言偏好同步到 <html>。在根组件调用一次。 */
export function usePreferenceEffects() {
  const language = usePreferencesStore((s) => s.language);
  const themeMode = usePreferencesStore((s) => s.themeMode);
  const accent = usePreferencesStore((s) => s.accent);
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
  }, [themeMode, systemDark]);
  useEffect(() => {
    document.documentElement.dataset.accent = accent;
  }, [accent]);
  useEffect(() => {
    const mobile = window.matchMedia("(max-width: 720px)");
    const update = () => {
      const styles = getComputedStyle(document.documentElement);
      const color = styles
        .getPropertyValue(mobile.matches ? "--xc-panel" : "--xc-bg")
        .trim();
      if (color)
        document
          .querySelector('meta[name="theme-color"]')
          ?.setAttribute("content", color);
    };
    update();
    const frame = requestAnimationFrame(update);
    window.addEventListener("load", update);
    mobile.addEventListener("change", update);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("load", update);
      mobile.removeEventListener("change", update);
    };
  }, [themeMode, systemDark, accent]);
  useEffect(() => {
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  }, [language]);
  return language;
}
