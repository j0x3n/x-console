/*
 * 便签弹窗（Google Keep 的样子）用：图片放在最上面，正文是纯文字。
 * 只把“整行只有一张图”的行当成图片，其他内容原样留在正文里。
 */

export interface MemoImage {
  /** 原来那一行 Markdown */
  line: string;
  src: string;
  alt: string;
}

const IMAGE_LINE = /^!\[([^\]]*)\]\(\s*<?([^\s)>]+)>?(?:\s+"[^"]*")?\s*\)$/;

export function splitMemo(body: string): { images: MemoImage[]; text: string } {
  const images: MemoImage[] = [];
  const rest: string[] = [];
  for (const line of body.split("\n")) {
    const m = IMAGE_LINE.exec(line.trim());
    if (m) images.push({ line: line.trim(), alt: m[1], src: m[2] });
    else rest.push(line);
  }
  if (images.length === 0) return { images, text: body };
  // 图片拿走后，开头多出来的空行去掉
  while (rest.length && rest[0].trim() === "") rest.shift();
  return { images, text: rest.join("\n") };
}

export function joinMemo(imageLines: string[], text: string): string {
  if (imageLines.length === 0) return text;
  return text
    ? `${imageLines.join("\n\n")}\n\n${text}`
    : imageLines.join("\n\n");
}
