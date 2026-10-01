import { describe, expect, it } from "vitest";
import { moduleOfCommandGroup, moduleOfPath } from "./modules";

describe("模块归属（B57）", () => {
  it("按地址第一段认模块", () => {
    expect(moduleOfPath("/notes/12")).toBe("notes");
    expect(moduleOfPath("/monitoring/subscriptions")).toBe("monitoring");
    expect(moduleOfPath("/")).toBeNull();
    expect(moduleOfPath("/settings/security")).toBeNull();
  });
  it("按命令分组认模块", () => {
    expect(moduleOfCommandGroup("笔记")).toBe("notes");
    expect(moduleOfCommandGroup("Issue")).toBe("projects");
    expect(moduleOfCommandGroup("AI")).toBeNull();
  });
});
