import { describe, expect, it } from "vitest";
import type { AiModel } from "../api";
import {
  canUseTools,
  currentMonth,
  filterModels,
  findModel,
  formatContext,
  formatCost,
  formatPrice,
  formatTokens,
  priceText,
} from "./models";

const model = (id: string, extra: Partial<AiModel> = {}): AiModel => ({
  providerId: 1,
  id,
  specSource: "exact",
  ...extra,
});

describe("model display", () => {
  it("formats context length", () => {
    expect(formatContext(131072)).toBe("128K");
    expect(formatContext(400_000)).toBe("400K");
    expect(formatContext(1_000_000)).toBe("1M");
    expect(formatContext(1_048_576)).toBe("1.0M");
    expect(formatContext(undefined)).toBe("");
  });

  it("formats prices per million tokens", () => {
    expect(formatPrice(0.15)).toBe("$0.15");
    expect(formatPrice(2.5)).toBe("$2.5");
    expect(formatPrice(0)).toBe("$0");
    expect(formatPrice(undefined)).toBe("");
    expect(priceText(model("a", { inputPrice: 3, outputPrice: 15 }))).toBe(
      "$3 / $15",
    );
    expect(priceText(model("a", { inputPrice: 3 }))).toBe("$3 / ?");
    expect(priceText(model("a"))).toBe("");
  });

  it("formats tokens and cost", () => {
    expect(formatTokens(999)).toBe("999");
    expect(formatTokens(12345)).toBe("12.3K");
    expect(formatTokens(1234567)).toBe("1.23M");
    expect(formatCost(1.5)).toBe("$1.50");
    expect(formatCost(undefined)).toBe("");
  });

  it("uses the local month", () => {
    expect(currentMonth(new Date(2026, 0, 5))).toBe("2026-01");
    expect(currentMonth(new Date(2026, 10, 30))).toBe("2026-11");
  });
});

describe("model choice", () => {
  const models = [
    model("gpt-5", { name: "GPT-5" }),
    model("deepseek-chat", { providerId: 2, toolCall: false }),
    model("local-x", { providerId: 2, specSource: "unknown" }),
  ];

  it("searches id and name without case", () => {
    expect(filterModels(models, "GPT").map((m) => m.id)).toEqual(["gpt-5"]);
    expect(filterModels(models, " deep ").map((m) => m.id)).toEqual([
      "deepseek-chat",
    ]);
    expect(filterModels(models, "")).toHaveLength(3);
  });

  it("finds a model by provider and id", () => {
    expect(findModel(models, { providerId: 2, model: "local-x" })?.id).toBe(
      "local-x",
    );
    expect(findModel(models, { providerId: 1, model: "local-x" })).toBe(
      undefined,
    );
    expect(findModel(models, undefined)).toBe(undefined);
  });

  it("only blocks models known to lack tool calls", () => {
    expect(models.map(canUseTools)).toEqual([true, false, true]);
  });
});
