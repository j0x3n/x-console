import { describe, expect, it } from "vitest";
import { isChunkLoadError } from "./chunkReload";

describe("isChunkLoadError", () => {
  it("knows the messages browsers give when a lazy file is gone", () => {
    expect(isChunkLoadError(new TypeError("Load failed"))).toBe(true);
    expect(
      isChunkLoadError(new TypeError("Importing a module script failed.")),
    ).toBe(true);
    expect(
      isChunkLoadError(
        new TypeError(
          "Failed to fetch dynamically imported module: /assets/a.js",
        ),
      ),
    ).toBe(true);
    expect(
      isChunkLoadError(new Error("Cannot read properties of undefined")),
    ).toBe(false);
    expect(isChunkLoadError(undefined)).toBe(false);
  });
});
