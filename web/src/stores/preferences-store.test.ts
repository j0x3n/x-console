import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("preferences store", () => {
  let storage: Map<string, string>;
  beforeEach(() => {
    vi.resetModules();
    storage = new Map();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("migrates old preferences and persists updates under X Console keys", async () => {
    storage.set("xcc-language", "en");
    storage.set("xcc-theme", "dark");
    const { usePreferencesStore } = await import("./preferences-store");
    const store = usePreferencesStore.getState();
    expect(store.language).toBe("en");
    expect(store.themeMode).toBe("dark");
    expect(storage.get("x-console-language")).toBe("en");
    store.setLanguage("zh");
    store.setThemeMode("light");
    expect(storage.get("x-console-language")).toBe("zh");
    expect(storage.get("x-console-theme")).toBe("light");
  });

  it("falls back to defaults for unknown values", async () => {
    storage.set("x-console-language", "fr");
    storage.set("x-console-theme", "blue");
    const { usePreferencesStore } = await import("./preferences-store");
    expect(usePreferencesStore.getState().language).toBe("zh");
    expect(usePreferencesStore.getState().themeMode).toBe("system");
  });
});
