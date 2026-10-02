import { describe, expect, it } from "vitest";
import { pickQuote } from "./quote";

const q = (id: number, pinned = false) => ({
  id,
  pinned,
  title: `标题 ${id}`,
  excerpt: `名言 ${id}`,
});

describe("B89 daily quote", () => {
  const list = [q(1, true), q(2, true), q(3), q(4)];
  it("is off by default and when there is nothing", () => {
    expect(pickQuote(list, "off", "2026-10-02", 0)).toBeNull();
    expect(pickQuote([], "daily", "2026-10-02", 0)).toBeNull();
  });
  it("fixed shows the first pinned one, or the newest", () => {
    expect(pickQuote(list, "fixed", "2026-10-02", 0.9)?.id).toBe(1);
    expect(pickQuote([q(3), q(4)], "fixed", "2026-10-02", 0)?.id).toBe(3);
  });
  it("daily stays the same within a day", () => {
    const a = pickQuote(list, "daily", "2026-10-02", 0.1);
    const b = pickQuote(list, "daily", "2026-10-02", 0.9);
    expect(a?.id).toBe(b?.id);
  });
  it("refresh follows the random number", () => {
    expect(pickQuote(list, "refresh", "", 0)?.id).toBe(1);
    expect(pickQuote(list, "refresh", "", 0.99)?.id).toBe(4);
  });
});
