import { describe, expect, it } from "vitest";
import type { Monitor, Subscription } from "../monitoring/api";
import { attentionItems } from "./attention";

const mon = (p: Partial<Monitor>) =>
  ({
    id: 1,
    kind: "http",
    name: "x",
    enabled: true,
    lastStatus: "up",
    ...p,
  }) as Monitor;
const sub = (p: Partial<Subscription>) =>
  ({ id: 1, name: "s", daysLeft: 30, ...p }) as Subscription;

describe("今日页监控提醒（B59）", () => {
  it("挑出要注意的，严重的在前", () => {
    const items = attentionItems(
      [
        mon({ id: 1, name: "官网", lastStatus: "down" }),
        mon({ id: 2, name: "正常网站" }),
        mon({ id: 3, kind: "tls", name: "证书", daysLeft: 10.5 }),
        mon({ id: 4, kind: "domain", name: "域名", daysLeft: 200 }),
        mon({ id: 5, kind: "tls", name: "停用", daysLeft: 1, enabled: false }),
      ],
      [sub({ id: 9, name: "ECS", daysLeft: 2 }), sub({ id: 8, daysLeft: 20 })],
    );
    expect(items.map((x) => [x.name, x.tone, x.days])).toEqual([
      ["官网", "danger", undefined],
      ["ECS", "danger", 2],
      ["证书", "warn", 10],
    ]);
  });
  it("都正常时为空", () => {
    expect(attentionItems([mon({})], [sub({})])).toEqual([]);
  });
});
