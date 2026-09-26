import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AutoSaver, type SaveState } from "./autosave";
import { defaultReminderTime, noteTitle, parseTags, sameTags, snippetParts } from "./logic";

describe("AutoSaver", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function setup(save = vi.fn(async (_draft: string) => {})) {
    const states: SaveState[] = [];
    const saver = new AutoSaver<string>({ delay: 800, retryDelay: 3000, save, onState: (s) => states.push(s) });
    return { saver, save, states };
  }

  it("saves only the last draft after typing stops", async () => {
    const { saver, save, states } = setup();
    saver.change("a");
    await vi.advanceTimersByTimeAsync(500);
    saver.change("ab");
    await vi.advanceTimersByTimeAsync(500);
    saver.change("abc");
    expect(save).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(800);
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("abc");
    expect(states.at(-1)).toBe("saved");
    expect(saver.dirty).toBe(false);
  });

  it("saves again when typing continues during a save", async () => {
    let release!: () => void;
    const save = vi.fn(
      (_draft: string) => new Promise<void>((resolve) => (release = resolve)),
    );
    const { saver } = setup(save);
    saver.change("one");
    await vi.advanceTimersByTimeAsync(800);
    expect(save).toHaveBeenLastCalledWith("one");
    saver.change("one two");
    await vi.advanceTimersByTimeAsync(800); // still waiting for the first save
    expect(save).toHaveBeenCalledTimes(1);
    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith("one two");
    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(saver.dirty).toBe(false);
  });

  it("keeps the draft and retries after a failure", async () => {
    const save = vi.fn(async (_draft: string) => {});
    save.mockRejectedValueOnce(new Error("offline"));
    const { saver, states } = setup(save);
    saver.change("keep me");
    await vi.advanceTimersByTimeAsync(800);
    expect(states.at(-1)).toBe("error");
    expect(saver.unsaved).toBe("keep me");
    await vi.advanceTimersByTimeAsync(3000);
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith("keep me");
    expect(states.at(-1)).toBe("saved");
  });

  it("flush saves immediately", async () => {
    const { saver, save } = setup();
    saver.change("now");
    await saver.flush();
    expect(save).toHaveBeenCalledWith("now");
    await vi.advanceTimersByTimeAsync(2000);
    expect(save).toHaveBeenCalledTimes(1);
  });
});

describe("notes helpers", () => {
  it("splits search snippets", () => {
    expect(snippetParts("…学习了数据库索引")).toEqual([
      { text: "…学习了", hit: false },
      { text: "数据库", hit: true },
      { text: "索引", hit: false },
    ]);
    expect(snippetParts("plain")).toEqual([{ text: "plain", hit: false }]);
    expect(snippetParts("ab")).toEqual([
      { text: "a", hit: true },
      { text: "b", hit: true },
    ]);
  });
  it("falls back to the first line for titles", () => {
    expect(noteTitle(" Title ", "body")).toBe("Title");
    expect(noteTitle("", "\n# 买菜\n- 牛奶")).toBe("买菜");
    expect(noteTitle("", "")).toBe("");
  });
  it("parses tag input", () => {
    expect(parseTags("#work, idea  work，生活")).toEqual(["work", "idea", "生活"]);
    expect(sameTags(["a", "b"], ["b", "a"])).toBe(true);
    expect(sameTags(["a"], ["a", "b"])).toBe(false);
  });
  it("defaults reminders to tomorrow 9:00", () => {
    expect(defaultReminderTime(new Date(2026, 8, 30, 22, 15))).toBe("2026-10-01T09:00");
  });
});
