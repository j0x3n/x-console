import { describe, expect, it } from "vitest";
import { trafficBytes } from "./TodayTrafficCard";
describe("proxy traffic units", () => {
  it("uses decimal GB and MB", () => {
    expect(trafficBytes(0)).toBe("0.0 MB");
    expect(trafficBytes(13500000000)).toBe("13.50 GB");
  });
});
