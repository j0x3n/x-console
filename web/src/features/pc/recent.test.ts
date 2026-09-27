import { describe, expect, it } from "vitest";
import { loadRecent, pushRecent } from "./recent";

describe("recent targets", () => {
  it("keeps the newest first without duplicates", () => {
    let list: string[] = [];
    for (const v of ["a", "b", "a", "c", "d", "e", "f", "g"])
      list = pushRecent(list, v);
    expect(list).toEqual(["g", "f", "e", "d", "c", "a"]);
  });
  it("survives missing localStorage", () => {
    expect(loadRecent()).toEqual([]);
  });
});
