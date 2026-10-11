import { describe, expect, it } from "vitest";
import type { InboxItem } from "./api";
import { formFrom, fromLocalInput, toLocalInput } from "./format";

const item = (over: Partial<InboxItem>): InboxItem => ({
  id: 1,
  text: "买牛奶\n还有鸡蛋",
  source: "web",
  status: "open",
  suggestState: "off",
  createdAt: "2026-11-11T00:00:00Z",
  ...over,
});

describe("时间输入框的转换", () => {
  it("来回转换不变", () => {
    const local = toLocalInput("2026-11-12T15:00:00+08:00");
    expect(local).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
    const back = fromLocalInput(local);
    expect(new Date(back).getTime()).toBe(
      new Date("2026-11-12T15:00:00+08:00").getTime(),
    );
  });
  it("空值和不合法的值给空字符串", () => {
    expect(toLocalInput("")).toBe("");
    expect(toLocalInput(undefined)).toBe("");
    expect(toLocalInput("不是时间")).toBe("");
    expect(fromLocalInput("")).toBe("");
    expect(fromLocalInput("乱写")).toBe("");
  });
  it("带本地时区偏移", () => {
    expect(fromLocalInput("2026-11-12T15:00")).toMatch(
      /T15:00:00[+-]\d{2}:\d{2}$/,
    );
  });
});

describe("表单初始值", () => {
  it("有建议就用建议，拿不准当笔记", () => {
    const f = formFrom(
      item({
        suggestState: "ready",
        suggestion: {
          kind: "reminder",
          title: "打电话",
          body: "",
          at: "2026-11-12T15:00:00+08:00",
          url: "",
          reason: "",
        },
      }),
    );
    expect(f.kind).toBe("reminder");
    expect(f.title).toBe("打电话");
    expect(f.at).not.toBe("");
    const none = formFrom(
      item({
        suggestState: "ready",
        suggestion: {
          kind: "none",
          title: "x",
          body: "",
          at: "",
          url: "",
          reason: "",
        },
      }),
    );
    expect(none.kind).toBe("note");
  });
  it("没有建议时用原文第一行做标题，全文做正文", () => {
    const f = formFrom(item({}));
    expect(f.title).toBe("买牛奶");
    expect(f.body).toBe("买牛奶\n还有鸡蛋");
    expect(f.kind).toBe("note");
  });
});
