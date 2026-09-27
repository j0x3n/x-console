import { describe, expect, it } from "vitest";
import { registerTap, vaultPasswordError } from "./logic";

describe("registerTap", () => {
  it("fires on the third tap within two seconds", () => {
    let taps: number[] = [];
    let fired = false;
    for (const at of [0, 300, 600]) {
      const r = registerTap(taps, at);
      taps = r.taps;
      fired = r.fire;
    }
    expect(fired).toBe(true);
    expect(taps).toEqual([]);
  });

  it("drops taps older than two seconds", () => {
    let taps: number[] = [];
    let fired = false;
    for (const at of [0, 1500, 2100]) {
      const r = registerTap(taps, at);
      taps = r.taps;
      fired = r.fire;
    }
    expect(fired).toBe(false);
    expect(taps).toEqual([1500, 2100]);
    expect(registerTap(taps, 2200).fire).toBe(true);
  });
});

describe("vaultPasswordError", () => {
  it("checks length and match", () => {
    expect(vaultPasswordError("12345", "12345")).toMatch("至少 6 位");
    expect(vaultPasswordError("123456", "123457")).toMatch("不一致");
    expect(vaultPasswordError("123456", "123456")).toBe("");
  });
});
