import { describe, expect, it } from "vitest";
import {
  guessProvider,
  hasRemoteImages,
  mailDocument,
  mailTime,
  reminderTimeFor,
  replaceCid,
  senderName,
  stripTrackingPixels,
  textToHtml,
} from "./logic";

describe("邮件（B53）", () => {
  it("认出外部图片", () => {
    expect(hasRemoteImages('<img src="https://t.co/p.gif">')).toBe(true);
    expect(
      hasRemoteImages('<div style="background:url(http://a/b.png)">'),
    ).toBe(true);
    expect(hasRemoteImages('<img src="cid:logo">')).toBe(false);
  });
  it("去掉追踪打开的 1 像素图片，正常图片保留", () => {
    const keep = [
      '<img src="https://a/logo.png">',
      '<img src="https://a/b.png" width="600" height="1">',
      '<img src="https://a/b.png" width="100%" height="1">',
      '<img src="https://a/b.png" style="max-width:1px;width:300px">',
      '<img src="https://a/b.png" data-width="1" data-height="1">',
    ];
    for (const tag of keep) expect(stripTrackingPixels(tag)).toBe(tag);
    const drop = [
      '<img src="https://t/o.gif" width="1" height="1">',
      "<img src='https://t/o.gif' height=1 width=1 />",
      '<img src="https://t/o.gif" width="0">',
      '<img src="https://t/o.gif" style="width:1px;height:1px">',
      '<img src="https://t/o.gif" style="display: none">',
      '<IMG SRC="https://t/o.gif" STYLE="visibility:hidden">',
    ];
    for (const tag of drop)
      expect(stripTrackingPixels(`<p>a</p>${tag}<p>b</p>`)).toBe(
        "<p>a</p><p>b</p>",
      );
  });
  it("cid 换成附件地址", () => {
    const html = replaceCid(
      '<img src="cid:logo@x"><img src="cid:none">',
      [{ index: 2, contentId: "<logo@x>" }],
      (i) => `/att/${i}`,
    );
    expect(html).toBe('<img src="/att/2"><img src="cid:none">');
  });
  it("默认不放开外部图片", () => {
    const off = mailDocument("<p>hi</p>", {
      origin: "https://x.im",
      showImages: false,
    });
    expect(off).toContain("img-src 'self' https://x.im data:;");
    expect(off).toContain("default-src 'none'");
    const on = mailDocument("<p>hi</p>", {
      origin: "https://x.im",
      showImages: true,
    });
    expect(on).toContain("data: https: http:;");
  });
  it("纯文字转义并加链接", () => {
    expect(textToHtml("a<b> https://x.im/p")).toBe(
      '<pre style="font:inherit;margin:0">a&lt;b&gt; <a href="https://x.im/p">https://x.im/p</a></pre>',
    );
  });
  it("发件人和时间", () => {
    expect(senderName({ name: " 阿里云 ", address: "no@aliyun.com" })).toBe(
      "阿里云",
    );
    expect(senderName({ address: "a@b.c" })).toBe("a@b.c");
    const now = new Date(2026, 9, 1, 15, 0);
    expect(mailTime(new Date(2026, 9, 1, 9, 5).toISOString(), now)).toBe(
      "09:05",
    );
    expect(mailTime(new Date(2026, 8, 3).toISOString(), now)).toBe("9月3日");
    expect(mailTime(new Date(2025, 0, 2).toISOString(), now)).toBe("2025/1/2");
    expect(guessProvider("x@Gmail.com")).toBe("gmail");
    expect(guessProvider("x@xcc.im")).toBe("other");
  });
});

describe("截止日的提醒时间（B136）", () => {
  const now = new Date(2026, 9, 15, 10, 0, 0); // 10 月 15 日 10:00
  it("定在前一天早上 9 点", () => {
    expect(reminderTimeFor("2026-10-20", now)).toEqual(
      new Date(2026, 9, 19, 9, 0, 0),
    );
  });
  it("前一天已经过了就定在当天 9 点", () => {
    expect(reminderTimeFor("2026-10-16", now)).toEqual(
      new Date(2026, 9, 16, 9, 0, 0),
    );
    expect(
      reminderTimeFor("2026-10-16", new Date(2026, 9, 15, 20, 0, 0)),
    ).toEqual(new Date(2026, 9, 16, 9, 0, 0));
  });
  it("当天 9 点也过了，或者日期不对，返回空", () => {
    expect(reminderTimeFor("2026-10-15", now)).toBeNull();
    expect(reminderTimeFor("2026-10-01", now)).toBeNull();
    expect(reminderTimeFor("明天", now)).toBeNull();
  });
});
