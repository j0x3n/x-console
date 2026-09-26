import { create } from "zustand";
import { resolveUpdate } from "./update";
import type { Language, Setter, ThemeMode } from "../types/domain";

function readPreference(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function readBrandPreference(name: string): string | null {
  const key = `x-console-${name}`;
  const current = readPreference(key);
  if (current !== null) return current;
  const previous = readPreference(`xcc-${name}`);
  if (previous !== null) savePreference(key, previous);
  return previous;
}
function savePreference(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Preferences still work in memory when storage is unavailable. */
  }
}
interface PreferencesState {
  language: Language;
  themeMode: ThemeMode;
  setLanguage: Setter<Language>;
  setThemeMode: Setter<ThemeMode>;
}
const savedTheme = readBrandPreference("theme");
export const usePreferencesStore = create<PreferencesState>()((set) => ({
  language: readBrandPreference("language") === "en" ? "en" : "zh",
  themeMode:
    savedTheme === "dark" || savedTheme === "light" ? savedTheme : "system",
  setLanguage: (update) =>
    set((state) => {
      const language = resolveUpdate(update, state.language);
      savePreference("x-console-language", language);
      return { language };
    }),
  setThemeMode: (update) =>
    set((state) => {
      const themeMode = resolveUpdate(update, state.themeMode);
      savePreference("x-console-theme", themeMode);
      return { themeMode };
    }),
}));
