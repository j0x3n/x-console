import { describe, expect, it } from "vitest";
import { zhConflicts } from "./i18n";

describe("zh dictionary", () => {
  it("has no key that means different things in different modules", () => {
    import.meta.glob("../features/*/i18n.ts", { eager: true });
    expect(zhConflicts()).toEqual([]);
  });
});
