import { describe, expect, it } from "vitest";
import { moduleForPath } from "./Sidebar";

describe("moduleForPath", () => {
  it("finds the module of a page", () => {
    expect(moduleForPath("/")).toBe("/");
    expect(moduleForPath("/projects")).toBe("/projects");
    expect(moduleForPath("/projects/XC")).toBe("/projects");
    expect(moduleForPath("/servers/12/files")).toBe("/servers");
  });

  it("counts the daily brief as the today page", () => {
    expect(moduleForPath("/calendar/briefs")).toBe("/");
    expect(moduleForPath("/calendar")).toBe("/calendar");
  });

  it("does not match a module by prefix alone", () => {
    expect(moduleForPath("/projectsx")).toBeUndefined();
    expect(moduleForPath("/settings")).toBeUndefined();
  });
});
