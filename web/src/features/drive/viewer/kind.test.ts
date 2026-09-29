import { describe, expect, it } from "vitest";
import {
  canEdit,
  clampZoom,
  concatBytes,
  decodeUtf8,
  neighbor,
  splitChunk,
  viewerKind,
} from "./kind";

const file = (name: string, size = 100, mime?: string) => ({
  name,
  size,
  mime,
  isDir: false,
});

describe("viewerKind", () => {
  it("picks the viewer from mime and extension", () => {
    expect(viewerKind(file("a.png"))).toBe("image");
    expect(viewerKind(file("a.mp4"))).toBe("video");
    expect(viewerKind(file("a.mp3"))).toBe("audio");
    expect(viewerKind(file("a.pdf"))).toBe("pdf");
    expect(viewerKind(file("README.md"))).toBe("markdown");
    expect(viewerKind(file("config.yaml"))).toBe("text");
    expect(viewerKind(file("app.log"))).toBe("log");
    expect(viewerKind(file("a.zip"))).toBe("other");
    expect(viewerKind(file("a.bin", 10, "application/octet-stream"))).toBe(
      "other",
    );
  });

  it("opens big text files as logs", () => {
    expect(viewerKind(file("dump.json", 20 * 1024 * 1024))).toBe("log");
  });
});

describe("canEdit", () => {
  it("allows text up to 10 MB", () => {
    expect(canEdit(file("a.yaml"))).toBe(true);
    expect(canEdit(file("notes", 10, "text/plain; charset=utf-8"))).toBe(true);
    expect(canEdit(file("a.png"))).toBe(false);
    expect(canEdit(file("a.zip"))).toBe(false);
    expect(canEdit(file("a.txt", 11 * 1024 * 1024))).toBe(false);
    expect(canEdit({ ...file("dir"), isDir: true })).toBe(false);
  });
});

describe("decodeUtf8", () => {
  it("rejects bytes that are not UTF-8", () => {
    expect(decodeUtf8(new TextEncoder().encode("你好"))).toBe("你好");
    expect(decodeUtf8(new Uint8Array([0xc4, 0xe3, 0xba, 0xc3]))).toBeNull();
  });
});

describe("neighbor", () => {
  const items = [{ id: 1 }, { id: 2 }, { id: 3 }];
  it("stops at both ends", () => {
    expect(neighbor(items, 2, 1)).toBe(2);
    expect(neighbor(items, 2, -1)).toBe(0);
    expect(neighbor(items, 3, 1)).toBe(-1);
    expect(neighbor(items, 1, -1)).toBe(-1);
    expect(neighbor(items, 9, 1)).toBe(-1);
  });
});

describe("splitChunk", () => {
  const enc = (s: string) => new TextEncoder().encode(s);
  it("keeps the partial first line for the earlier chunk", () => {
    const { head, lines } = splitChunk(enc("尾巴\nb\nc\n"), false);
    expect(new TextDecoder().decode(head)).toBe("尾巴");
    expect(lines).toEqual(["b", "c"]);
    expect(splitChunk(enc("a\nb"), true).lines).toEqual(["a", "b"]);
    expect(splitChunk(enc(""), true).lines).toEqual([]);
  });

  it("joins a multi-byte character split across chunks", () => {
    const all = enc("前一行\n中文日志\n");
    // 在“中”字中间切开。
    const cut = enc("前一行\n").length + 1;
    const later = splitChunk(all.slice(cut), false);
    expect(later.lines).toEqual([]);
    const earlier = splitChunk(
      concatBytes(all.slice(0, cut), later.head),
      true,
    );
    expect(earlier.lines).toEqual(["前一行", "中文日志"]);
  });
});

describe("clampZoom", () => {
  it("keeps zoom in range", () => {
    expect(clampZoom(20)).toBe(8);
    expect(clampZoom(0.01)).toBe(0.1);
    expect(clampZoom(1.234)).toBe(1.23);
  });
});
