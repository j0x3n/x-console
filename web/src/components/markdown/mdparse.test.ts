import { describe, expect, it } from "vitest";
import { imageSources, parseInline, parseMarkdown, safeHref, toggleTask } from "./mdparse";

describe("parseMarkdown", () => {
  it("parses headings, paragraphs and rules", () => {
    const blocks = parseMarkdown("# Title\n\nline one\nline two\n\n---");
    expect(blocks.map((b) => b.type)).toEqual(["heading", "paragraph", "hr"]);
    expect(blocks[1]).toMatchObject({
      children: [
        { type: "text", text: "line one" },
        { type: "br" },
        { type: "text", text: "line two" },
      ],
    });
  });

  it("parses fenced code without touching its content", () => {
    const blocks = parseMarkdown("```go\nfunc **x**() {}\n```\nafter");
    expect(blocks[0]).toEqual({ type: "code", lang: "go", text: "func **x**() {}" });
    expect(blocks[1].type).toBe("paragraph");
  });

  it("parses task lists and nested lists", () => {
    const blocks = parseMarkdown("- [x] done\n- [ ] todo\n  - child\n1. one\n2. two");
    expect(blocks).toHaveLength(2);
    const list = blocks[0];
    if (list.type !== "list") throw new Error("not a list");
    expect(list.items.map((i) => i.checked)).toEqual([true, false]);
    expect(list.items[1].blocks.map((b) => b.type)).toEqual(["paragraph", "list"]);
    expect(blocks[1]).toMatchObject({ type: "list", ordered: true, start: 1 });
  });

  it("parses quotes", () => {
    const blocks = parseMarkdown("> quoted **text**");
    expect(blocks[0]).toMatchObject({ type: "quote", blocks: [{ type: "paragraph" }] });
  });
});

describe("parseInline", () => {
  it("parses emphasis, code and links", () => {
    expect(parseInline("a **b** *c* ~~d~~ `e`").map((n) => n.type)).toEqual([
      "text", "strong", "text", "em", "text", "del", "text", "code",
    ]);
    expect(parseInline("[XC-1](/projects/XC/1)")).toEqual([
      { type: "link", href: "/projects/XC/1", children: [{ type: "text", text: "XC-1" }] },
    ]);
    expect(parseInline("see https://example.com.")[1]).toMatchObject({ type: "link", href: "https://example.com" });
  });

  it("keeps snake_case words intact", () => {
    expect(parseInline("use snake_case_name here")).toEqual([{ type: "text", text: "use snake_case_name here" }]);
  });

  it("drops unsafe link targets", () => {
    expect(parseInline("[x](javascript:alert(1))")[0]).toMatchObject({ type: "text" });
    expect(safeHref("//evil.com")).toBeNull();
    expect(safeHref("mailto:a@b.c")).toBe("mailto:a@b.c");
  });
});

describe("images and tasks", () => {
  it("parses images and keeps unsafe ones as text", () => {
    expect(parseInline("![cat](/api/v1/notes/attachments/3)")).toEqual([
      { type: "image", src: "/api/v1/notes/attachments/3", alt: "cat" },
    ]);
    expect(parseInline("![x](javascript:alert(1))")[0]).toMatchObject({ type: "text" });
    expect(parseInline("![上传中…]()")).toEqual([{ type: "text", text: "上传中…" }]);
    expect(parseInline("[link](/notes/1)")[0]).toMatchObject({ type: "link", href: "/notes/1" });
  });

  it("toggles the nth task and skips code blocks", () => {
    const src = "- [ ] a\n```\n- [ ] not a task\n```\n- [x] b\n  - [ ] c";
    expect(toggleTask(src, 0)).toBe(src.replace("- [ ] a", "- [x] a"));
    expect(toggleTask(src, 1)).toBe(src.replace("- [x] b", "- [ ] b"));
    expect(toggleTask(src, 2)).toBe(src.replace("  - [ ] c", "  - [x] c"));
    expect(toggleTask(src, 3)).toBe(src);
  });

  it("lists image sources", () => {
    expect(imageSources("a ![x](/a.png) b ![y](https://e.com/b.png \"t\") ![z](bad:1)")).toEqual([
      "/a.png",
      "https://e.com/b.png",
    ]);
  });
});
