// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import {
  DEFAULT_PARTS,
  exportDate,
  imageFileName,
  MAX_PIXELS,
  MAX_SCALE,
  pickScale,
  previewSelection,
  readParts,
  saveParts,
  SHEET_WIDTH,
  textareaSelection,
} from "./imageExport";

afterEach(() => {
  localStorage.clear();
  document.body.innerHTML = "";
  window.getSelection()?.removeAllRanges();
});

describe("B242 导出图片的选项", () => {
  it("没存过时用默认值：标题和日期勾选，其余不勾", () => {
    expect(readParts()).toEqual({
      title: true,
      date: true,
      tags: false,
      hidden: false,
      brand: false,
    });
  });

  it("存过的选项下次读出来一样，坏数据退回默认", () => {
    saveParts({ ...DEFAULT_PARTS, title: false, brand: true });
    expect(readParts()).toEqual({
      ...DEFAULT_PARTS,
      title: false,
      brand: true,
    });
    localStorage.setItem("xc.notes.imageExport", "{bad");
    expect(readParts()).toEqual(DEFAULT_PARTS);
    localStorage.setItem("xc.notes.imageExport", '{"tags":"yes","date":false}');
    expect(readParts()).toEqual({ ...DEFAULT_PARTS, date: false });
  });
});

describe("B242 导出倍数", () => {
  it("不长时用 2.7 倍，宽 1080", () => {
    expect(pickScale(800)).toBe(MAX_SCALE);
    expect(SHEET_WIDTH * MAX_SCALE).toBe(1080);
  });

  it("长了降倍，总像素不超过上限", () => {
    const height = 8000;
    const scale = pickScale(height)!;
    expect(scale).toBeLessThan(MAX_SCALE);
    expect(scale).toBeGreaterThanOrEqual(1.5);
    expect(SHEET_WIDTH * scale * height * scale).toBeLessThanOrEqual(
      MAX_PIXELS,
    );
  });

  it("降到 1.5 倍还放不下时返回 null", () => {
    expect(pickScale(20000)).toBeNull();
  });
});

describe("B242 文件名和日期", () => {
  it("标题里不能用的字符换成横线，空标题用默认名", () => {
    const day = new Date(2026, 9, 17, 9, 0);
    expect(imageFileName("周报 a/b:c", day, "笔记")).toBe(
      "周报 a-b-c-20261017.png",
    );
    expect(imageFileName("   ", day, "笔记")).toBe("笔记-20261017.png");
  });

  it("日期按本地时间写成 2026-10-17", () => {
    expect(exportDate(new Date(2026, 9, 17, 23, 30).toISOString())).toBe(
      "2026-10-17",
    );
    expect(exportDate("bad")).toBe("");
  });
});

describe("B242 选中的部分", () => {
  it("编辑框有焦点时取选中的原文，没焦点时不算", () => {
    const el = document.createElement("textarea");
    el.value = "第一行\n**第二行**\n第三行";
    document.body.appendChild(el);
    el.focus();
    el.setSelectionRange(4, 11);
    expect(textareaSelection(el)).toBe("**第二行**");
    el.blur();
    expect(textareaSelection(el)).toBe("");
    expect(textareaSelection(null)).toBe("");
  });

  it("阅读模式下选中的文字换回原文，扩到整行", () => {
    const body = "开头一段文字\n这是**加粗的一句话**后面还有\n结尾一段文字";
    const root = document.createElement("div");
    root.innerHTML =
      "<p>开头一段文字</p><p>这是<strong>加粗的一句话</strong>后面还有</p><p>结尾一段文字</p>";
    document.body.appendChild(root);
    const strong = root.querySelector("strong")!.firstChild!;
    const range = document.createRange();
    range.setStart(strong, 1);
    range.setEnd(strong, 4);
    window.getSelection()!.addRange(range);
    expect(previewSelection(body, root)).toBe("这是**加粗的一句话**后面还有");
  });

  it("选区不在正文里时返回空", () => {
    const root = document.createElement("div");
    root.textContent = "正文";
    const other = document.createElement("p");
    other.textContent = "别的地方的文字";
    document.body.append(root, other);
    const range = document.createRange();
    range.selectNodeContents(other);
    window.getSelection()!.addRange(range);
    expect(previewSelection("正文", root)).toBe("");
  });
});
