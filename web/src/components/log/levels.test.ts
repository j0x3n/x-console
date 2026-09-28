import { describe, expect, it } from "vitest";
import {
  detectLevel,
  levelFromPriority,
  matchesLevel,
  splitHighlight,
} from "./levels";

describe("log levels", () => {
  it("detects levels from keywords", () => {
    expect(detectLevel("2026/09/28 ERROR connection refused")).toBe("error");
    expect(detectLevel("panic: runtime error")).toBe("error");
    expect(detectLevel("[warn] disk almost full")).toBe("warn");
    expect(detectLevel("level=info msg=started")).toBe("info");
    expect(detectLevel("DEBUG cache hit")).toBe("debug");
    expect(detectLevel("GET /health 200")).toBe("other");
    // 单词里包含 error 不算，比如 errorless 不会出现，但 terror 也不算
    expect(detectLevel("terrorist movie night")).toBe("other");
  });

  it("maps journal priorities", () => {
    expect(levelFromPriority(3)).toBe("error");
    expect(levelFromPriority(4)).toBe("warn");
    expect(levelFromPriority(6)).toBe("info");
    expect(levelFromPriority(7)).toBe("debug");
  });

  it("filters by level and above", () => {
    expect(matchesLevel("error", "warn")).toBe(true);
    expect(matchesLevel("info", "warn")).toBe(false);
    expect(matchesLevel("other", "all")).toBe(true);
    expect(matchesLevel("other", "debug")).toBe(false);
  });

  it("splits text for highlighting", () => {
    expect(splitHighlight("Error: error", "ERROR")).toEqual([
      { text: "Error", hit: true },
      { text: ": ", hit: false },
      { text: "error", hit: true },
    ]);
    expect(splitHighlight("abc", "")).toEqual([{ text: "abc", hit: false }]);
  });
});
