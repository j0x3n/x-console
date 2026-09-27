import { describe, expect, it } from "vitest";
import {
  attachmentMarkdown,
  countWords,
  dateGroup,
  insertBlock,
  prefixLines,
  wrapSelection,
} from "./logic";

describe("wrapSelection", () => {
  it("wraps the selection and keeps it selected", () => {
    expect(wrapSelection("hello world", 6, 11, "**")).toEqual({ text: "hello **world**", start: 8, end: 13 });
  });
  it("inserts a placeholder when nothing is selected", () => {
    expect(wrapSelection("ab", 1, 1, "*", "*", "text")).toEqual({ text: "a*text*b", start: 2, end: 6 });
  });
  it("removes the marks when they are already there", () => {
    expect(wrapSelection("hello **world**", 8, 13, "**")).toEqual({ text: "hello world", start: 6, end: 11 });
  });
});

describe("prefixLines", () => {
  it("adds a prefix to every selected line", () => {
    expect(prefixLines("a\nb\nc", 0, 3, "- ").text).toBe("- a\n- b\nc");
  });
  it("removes the prefix when all lines have it", () => {
    expect(prefixLines("- a\n- b", 0, 7, "- ").text).toBe("a\nb");
  });
  it("numbers ordered lists and replaces other list marks", () => {
    expect(prefixLines("- a\n- b", 0, 7, "", true).text).toBe("1. a\n2. b");
  });
  it("turns a bullet into a task", () => {
    expect(prefixLines("- a", 0, 3, "- [ ] ").text).toBe("- [ ] a");
  });
});

describe("insertBlock", () => {
  it("adds blank lines around a block in the middle of text", () => {
    const out = insertBlock("before after", 6, 7, "![x](/a)");
    expect(out.text).toBe("before\n\n![x](/a)\n\nafter");
    expect(out.start).toBe(16);
  });
  it("adds nothing at the start of an empty note", () => {
    expect(insertBlock("", 0, 0, "![x](/a)").text).toBe("![x](/a)");
  });
});

describe("dateGroup", () => {
  const now = new Date(2026, 8, 27, 10); // 周日
  it("groups by day, week and month", () => {
    expect(dateGroup(new Date(2026, 8, 27, 1), now)).toBe("today");
    expect(dateGroup(new Date(2026, 8, 26, 23), now)).toBe("yesterday");
    expect(dateGroup(new Date(2026, 8, 22), now)).toBe("week");
    expect(dateGroup(new Date(2026, 8, 20), now)).toBe("month");
    expect(dateGroup(new Date(2026, 7, 30), now)).toBe("earlier");
  });
});

describe("countWords", () => {
  it("counts Chinese characters and English words", () => {
    expect(countWords("你好 world, it's fine")).toBe(5);
    expect(countWords("")).toBe(0);
  });
});

describe("attachmentMarkdown", () => {
  it("uses image syntax for images but not svg", () => {
    expect(attachmentMarkdown({ name: "a.png", mime: "image/png", url: "/x" })).toBe("![a.png](/x)");
    expect(attachmentMarkdown({ name: "a.svg", mime: "image/svg+xml", url: "/x" })).toBe("[a.svg](/x)");
    expect(attachmentMarkdown({ name: "r[1].pdf", mime: "application/pdf", url: "/y" })).toBe("[r1.pdf](/y)");
  });
});
