import { apiFetch } from "../../api/client";
import type { components } from "../../api/gen/files";

/*
 * Markdown 编辑框贴图用的公共上传（B36，接口见 api/modules/files.yaml）。
 * 笔记有自己的附件接口，只共用这里的占位和插入文字。
 */

export type UploadScope = components["schemas"]["FileScope"];
export type UploadedFile = components["schemas"]["UploadedFile"];

/** 单张图片最大 20 MB，和服务端一致。 */
export const MAX_IMAGE_BYTES = 20 * 1024 * 1024;

/** 服务端只收这几种图片。 */
export const IMAGE_TYPES = [
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
];

export async function uploadFile(
  scope: UploadScope,
  file: File,
): Promise<UploadedFile> {
  const form = new FormData();
  form.append("file", file, file.name || "image.png");
  const res = await apiFetch(`/files?scope=${scope}`, {
    method: "POST",
    body: form,
  });
  return (await res.json()) as UploadedFile;
}

/** 上传中的占位文字，完成后换成真正的图片或链接。 */
export function uploadPlaceholder(name: string, id: string): string {
  return `![上传中 ${name.replace(/[[\]]/g, "")} ${id}…]()`;
}

/** 上传完成后插入的 Markdown：图片用 ![]()，其他文件用 []()。 */
export function attachmentMarkdown(a: {
  name: string;
  mime: string;
  url: string;
}): string {
  const name = a.name.replace(/[[\]]/g, "");
  // 图片、视频、音频用图片的写法，渲染时显示成图片或播放器（B74）
  const inline =
    (a.mime.startsWith("image/") && a.mime !== "image/svg+xml") ||
    a.mime.startsWith("video/") ||
    a.mime.startsWith("audio/");
  return inline ? `![${name}](${a.url})` : `[${name}](${a.url})`;
}

/** 公共上传的图片地址，形如 /api/v1/files/12。 */
const FILE_URL = /^\/api\/v1\/files\/\d+$/;

/** 显示用的地址：公共上传的图片取缩略图，点开再看原图。 */
export function thumbnailSrc(src: string): string {
  return FILE_URL.test(src) ? `${src}?thumb=1` : src;
}

export const isUploadedFile = (src: string) => FILE_URL.test(src);
