import type { MailAddress, MailAttachmentLike } from "./types";

/*
 * B53 邮件的纯函数：正文怎么安全地显示、发件人和时间怎么写。
 */

/** 正文里有没有外部图片。有的话阅读区显示“显示图片”按钮。 */
export function hasRemoteImages(html: string): boolean {
  return (
    /<img[^>]+src\s*=\s*["']?\s*https?:/i.test(html) ||
    /url\(\s*["']?https?:/i.test(html)
  );
}

/** 正文里的 cid:xxx 换成附件地址，内嵌图片才能显示。 */
export function replaceCid(
  html: string,
  attachments: MailAttachmentLike[],
  url: (index: number) => string,
): string {
  return html.replace(/cid:([^"'\s)>]+)/gi, (all, id: string) => {
    const a = attachments.find(
      (x) => x.contentId && x.contentId.replace(/^<|>$/g, "") === id,
    );
    return a ? url(a.index) : all;
  });
}

/**
 * 放进沙盒 iframe 的完整页面。用 CSP 拦住脚本和外部请求：
 * 默认只允许站内地址（内嵌图片）和 data: 图片，点“显示图片”后才放开外部图片。
 * 链接都在新窗口打开。
 */
export function mailDocument(
  html: string,
  opts: { origin: string; showImages: boolean },
): string {
  const img = `'self' ${opts.origin} data:${opts.showImages ? " https: http:" : ""}`;
  const csp = `default-src 'none'; img-src ${img}; style-src 'unsafe-inline'; font-src data:; media-src 'none'`;
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><base target="_blank"><style>
html,body{margin:0;padding:0;background:#fff;color:#1f1f1f;font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;word-break:break-word}
body{padding:4px 2px}img{max-width:100%;height:auto}table{max-width:100%}pre{white-space:pre-wrap}
</style></head><body>${html}</body></html>`;
}

/** 纯文字正文转成能放进 iframe 的 HTML，链接能点。 */
export function textToHtml(text: string): string {
  const esc = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
  const linked = esc.replace(
    /https?:\/\/[^\s<>"]+/g,
    (u) => `<a href="${u}">${u}</a>`,
  );
  return `<pre style="font:inherit;margin:0">${linked}</pre>`;
}

/** 发件人显示名：有名字用名字，没有用地址。 */
export function senderName(a: MailAddress | undefined): string {
  if (!a) return "";
  return a.name?.trim() || a.address;
}

/** 列表里的时间：今天写时间，今年写月日，更早写年月日。 */
export function mailTime(iso: string, now: Date): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  if (d.toDateString() === now.toDateString())
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  if (d.getFullYear() === now.getFullYear())
    return `${d.getMonth() + 1}月${d.getDate()}日`;
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`;
}

/** 常用邮箱的服务器地址，添加账号时自动填。 */
export const providerPresets = {
  gmail: { imapHost: "imap.gmail.com", imapPort: 993 },
  aliyun: { imapHost: "imap.qiye.aliyun.com", imapPort: 993 },
  other: { imapHost: "", imapPort: 993 },
} as const;

/** 按邮箱地址猜是哪家。 */
export function guessProvider(email: string): "gmail" | "aliyun" | "other" {
  const domain = email.split("@")[1]?.toLowerCase() ?? "";
  if (domain === "gmail.com" || domain === "googlemail.com") return "gmail";
  return "other";
}
