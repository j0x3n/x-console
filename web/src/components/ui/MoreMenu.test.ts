import { describe, expect, it } from "vitest";
import { menuPosition } from "./MoreMenu";

const view = { width: 1000, height: 800 };
const menu = { width: 180, height: 120 };

describe("menuPosition", () => {
  it("默认在按钮下方右对齐", () => {
    const b = { top: 100, bottom: 130, left: 500, right: 530 };
    expect(menuPosition(b, menu, view)).toEqual({ top: 134, left: 350 });
  });

  it("下方放不下时放到上方", () => {
    const b = { top: 720, bottom: 750, left: 500, right: 530 };
    expect(menuPosition(b, menu, view)).toEqual({ top: 596, left: 350 });
  });

  it("左边不超出窗口", () => {
    const b = { top: 100, bottom: 130, left: 0, right: 30 };
    expect(menuPosition(b, menu, view).left).toBe(8);
  });

  it("上下都放不下时贴着窗口底部", () => {
    const tall = { width: 180, height: 700 };
    const b = { top: 300, bottom: 330, left: 500, right: 530 };
    expect(menuPosition(b, tall, view).top).toBe(92);
  });
});
