import { describe, expect, it } from "vitest";
import {
  guessProvider,
  hasRemoteImages,
  mailDocument,
  mailTime,
  replaceCid,
  senderName,
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
