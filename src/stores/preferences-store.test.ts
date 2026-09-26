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

  it("restores xcc preferences and persists typed updates", async () => {
    storage.set("xcc-language", "en");
    storage.set("xcc-theme", "dark");
    storage.set("xcc-agent-modes", JSON.stringify({ Scout: "Ask first" }));
    const { usePreferencesStore } = await import("./preferences-store");
    const store = usePreferencesStore.getState();
    expect(store.language).toBe("en");
    expect(store.themeMode).toBe("dark");
    expect(store.agentModes.Scout).toBe("Ask first");
    store.setLanguage("zh");
    store.setThemeMode("light");
    store.setAgentModes((modes) => ({ ...modes, Echo: "Autopilot" }));
    expect(storage.get("xcc-language")).toBe("zh");
    expect(storage.get("xcc-theme")).toBe("light");
    expect(JSON.parse(storage.get("xcc-agent-modes")!)).toEqual({
      Scout: "Ask first",
      Echo: "Autopilot",
    });
  });

  it("rejects malformed or invalid saved agent modes", async () => {
    storage.set(
      "xcc-agent-modes",
      JSON.stringify({ Scout: "invalid", Echo: "Autopilot", Pilot: 42 }),
    );
    const { usePreferencesStore } = await import("./preferences-store");
    expect(usePreferencesStore.getState().agentModes).toEqual({
      Echo: "Autopilot",
    });
  });

  it("keeps preferences usable when browser storage throws", async () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    });
    const { usePreferencesStore } = await import("./preferences-store");
    usePreferencesStore.getState().setThemeMode("dark");
    expect(usePreferencesStore.getState().themeMode).toBe("dark");
    expect(usePreferencesStore.getState().language).toBe("zh");
  });
});
