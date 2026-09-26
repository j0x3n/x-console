import { create } from "zustand";
import { resolveUpdate } from "./update";
import type { AgentMode, Language, Setter, ThemeMode } from "../types/domain";

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
function readAgentModes(): Record<string, AgentMode> {
  try {
    const value: unknown = JSON.parse(
      readBrandPreference("agent-modes") || "{}",
    );
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    return Object.fromEntries(
      Object.entries(value).filter(
        (entry): entry is [string, AgentMode] =>
          entry[1] === "Suggest only" ||
          entry[1] === "Ask first" ||
          entry[1] === "Autopilot",
      ),
    );
  } catch {
    return {};
  }
}
interface PreferencesState {
  language: Language;
  themeMode: ThemeMode;
  agentModes: Record<string, AgentMode>;
  setLanguage: Setter<Language>;
  setThemeMode: Setter<ThemeMode>;
  setAgentModes: Setter<Record<string, AgentMode>>;
}
const savedTheme = readBrandPreference("theme");
export const usePreferencesStore = create<PreferencesState>()((set) => ({
  language: readBrandPreference("language") === "en" ? "en" : "zh",
  themeMode:
    savedTheme === "dark" || savedTheme === "light" ? savedTheme : "system",
  agentModes: readAgentModes(),
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
  setAgentModes: (update) =>
    set((state) => {
      const agentModes = resolveUpdate(update, state.agentModes);
      savePreference("x-console-agent-modes", JSON.stringify(agentModes));
      return { agentModes };
    }),
}));
