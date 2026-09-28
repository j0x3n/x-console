import { create } from "zustand";
import { resolveUpdate } from "./update";
import type { Accent, Language, Setter, ThemeMode } from "../types/domain";

export const accents: Accent[] = [
  "ember",
  "violet",
  "mint",
  "ocean",
  "rose",
  "graphite",
];

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
export interface PreferencesState {
  language: Language;
  /** dark 就是夜间模式“开”，light 是“关”，system 是“自动”。 */
  themeMode: ThemeMode;
  accent: Accent;
  setLanguage: Setter<Language>;
  setThemeMode: Setter<ThemeMode>;
  setAccent: (accent: Accent) => void;
}
const savedTheme = readBrandPreference("theme");
const savedAccent = readPreference("x-console-accent") as Accent | null;
export const usePreferencesStore = create<PreferencesState>()((set) => ({
  language: readBrandPreference("language") === "en" ? "en" : "zh",
  themeMode:
    savedTheme === "dark" || savedTheme === "light" ? savedTheme : "system",
  accent: savedAccent && accents.includes(savedAccent) ? savedAccent : "ember",
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
  setAccent: (accent) => {
    savePreference("x-console-accent", accent);
    set({ accent });
  },
}));
