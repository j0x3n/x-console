import { describe, expect, it } from "vitest";
import {
  cleanInput,
  emptyAutomation,
  formatCooldown,
  hasDangerous,
  schemaFields,
  triggerSummary,
  validateAutomation,
} from "./logic";

describe("schemaFields", () => {
  it("turns a flat schema into form fields", () => {
    const fields = schemaFields({
      type: "object",
      properties: {
        title: { type: "string" },
        body: { type: "string" },
        priority: { type: "string", enum: ["low", "high"] },
        estimate: { type: "integer" },
        pinned: { type: "boolean" },
      },
      required: ["title"],
    });
    expect(fields?.map((f) => [f.name, f.kind, f.required])).toEqual([
      ["title", "string", true],
      ["body", "text", false],
      ["priority", "enum", false],
      ["estimate", "integer", false],
      ["pinned", "boolean", false],
    ]);
  });
  it("gives up on arrays and nested objects", () => {
    expect(
      schemaFields({ type: "object", properties: { tags: { type: "array" } } }),
    ).toBeNull();
    expect(
      schemaFields({ type: "object", properties: { o: { type: "object" } } }),
    ).toBeNull();
  });
});

describe("cleanInput", () => {
  const fields = schemaFields({
    type: "object",
    properties: { n: { type: "integer" }, s: { type: "string" } },
  });
  it("drops empty strings and converts numbers, keeping templates", () => {
    expect(cleanInput({ n: "3", s: "" }, fields)).toEqual({ n: 3 });
    expect(cleanInput({ n: "{{trigger.data.cpu}}", s: "x" }, fields)).toEqual({
      n: "{{trigger.data.cpu}}",
      s: "x",
    });
    expect(cleanInput({ any: [1] }, null)).toEqual({ any: [1] });
  });
});

describe("rules", () => {
  it("validates before saving", () => {
    const a = emptyAutomation();
    expect(validateAutomation(a)).toEqual(["填一个名称", "至少加一个动作"]);
    const ok = {
      ...a,
      name: "早报",
      actions: [{ action: "brief.send", input: {} }],
    };
    expect(validateAutomation(ok)).toEqual([]);
    expect(
      validateAutomation({
        ...ok,
        trigger: { type: "schedule", cron: "0 9 *" },
      })[0],
    ).toMatch("5 段");
    expect(
      validateAutomation({ ...ok, trigger: { type: "event", topic: " " } }),
    ).toEqual(["填要订阅的事件主题"]);
  });
  it("summarizes triggers", () => {
    expect(
      triggerSummary({ type: "metric", metric: "cpu", op: ">", value: 90 }),
    ).toBe("任意服务器 CPU > 90%");
    expect(triggerSummary({ type: "ha_state", entityId: "light.a" })).toBe(
      "light.a 变成 任何状态",
    );
    expect(triggerSummary({ type: "webhook" })).toMatch("保存后");
  });
  it("finds dangerous steps", () => {
    const catalog = [
      {
        name: "scripts.run",
        title: "运行脚本",
        effect: "dangerous" as const,
        input: {},
      },
      {
        name: "notes.create",
        title: "新建笔记",
        effect: "write" as const,
        input: {},
      },
    ];
    expect(hasDangerous([{ action: "notes.create", input: {} }], catalog)).toBe(
      false,
    );
    expect(hasDangerous([{ action: "scripts.run", input: {} }], catalog)).toBe(
      true,
    );
  });
  it("formats cooldowns", () => {
    expect(formatCooldown(0)).toBe("不限制");
    expect(formatCooldown(600)).toBe("10 分钟");
    expect(formatCooldown(7200)).toBe("2 小时");
    expect(formatCooldown(45)).toBe("45 秒");
  });
});
