import { apiFetch } from "../../api/client";
import type { DriveItem } from "../drive/api";

/*
 * 扫描件和发票放在云盘的“证件档案”文件夹里（B115）。
 * 隐藏内容已解锁时放在隐藏空间，没解锁时放在普通文件夹。
 * 文件夹不存在就新建。档案里只记云盘文件的编号和名称。
 */
export const SCAN_FOLDER = "证件档案";

async function listFolders(hidden: boolean): Promise<DriveItem[]> {
  const res = await apiFetch(`/drive/items${hidden ? "?hidden=true" : ""}`);
  const body = (await res.json()) as { items: DriveItem[] };
  return body.items;
}

/** 找到或新建存扫描件的文件夹，返回它的编号。 */
export async function scanFolder(hidden: boolean): Promise<number> {
  const found = (await listFolders(hidden)).find(
    (i) => i.isDir && i.name === SCAN_FOLDER && !i.trashedAt,
  );
  if (found) return found.id;
  const res = await apiFetch("/drive/folders", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: SCAN_FOLDER, hidden }),
  });
  return ((await res.json()) as DriveItem).id;
}

/** 上传一个文件，返回云盘里的条目。 */
export async function uploadScan(
  file: File,
  hidden: boolean,
): Promise<DriveItem> {
  const parent = await scanFolder(hidden);
  const q = new URLSearchParams({ parent: String(parent) });
  if (hidden) q.set("hidden", "true");
  const form = new FormData();
  form.append("file", file, file.name);
  const res = await apiFetch(`/drive/upload?${q}`, {
    method: "POST",
    body: form,
  });
  const body = (await res.json()) as { items: DriveItem[] };
  if (!body.items[0]) throw new Error("上传失败");
  return body.items[0];
}
