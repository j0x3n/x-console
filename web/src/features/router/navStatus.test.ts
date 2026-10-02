import { describe, expect, it } from "vitest";
import { shortRate } from "./navStatus";
import { intervalLabel } from "./interval";

describe("B93 router nav", () => {
  it("shortens rates", () => {
    expect(shortRate(500)).toBe("500B");
    expect(shortRate(512 * 1024)).toBe("512K");
    expect(shortRate(3.2 * 1024 * 1024)).toBe("3.2M");
  });
  it("labels intervals", () => {
    expect(intervalLabel(1000)).toBe("每 1 秒");
    expect(intervalLabel(60_000)).toBe("每 1 分钟");
    expect(intervalLabel(0)).toBe("已暂停");
  });
});
