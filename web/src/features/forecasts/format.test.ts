import { describe, expect, it } from "vitest";
import { biasText, OUTCOME_LABEL, pct } from "./format";

describe("forecast format", () => {
  it("calls a small bias close and signs the rest", () => {
    expect(biasText(0)).toBe("close");
    expect(biasText(4.9)).toBe("close");
    expect(biasText(-4.9)).toBe("close");
    expect(biasText(12)).toBe("high");
    expect(biasText(-12)).toBe("low");
  });

  it("keeps the bar width inside 0 to 100", () => {
    expect(pct(-5)).toBe("0%");
    expect(pct(62.5)).toBe("62.5%");
    expect(pct(140)).toBe("100%");
  });

  it("words the outcome by kind", () => {
    expect(OUTCOME_LABEL.prediction.yes).not.toBe(OUTCOME_LABEL.decision.yes);
    expect(Object.keys(OUTCOME_LABEL.decision)).toEqual(["yes", "no", "void"]);
  });
});
