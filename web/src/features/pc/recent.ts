/*
 * 最近打开过的程序和网址，存在这台设备的 localStorage 里，
 * 只是个人便利，读写失败时当作没有。
 */
const KEY = "xc.pc.recentOpen";
const SELECTED = "xc.pc.host";
const MAX = 6;

export function pushRecent(list: string[], value: string): string[] {
  return [value, ...list.filter((v) => v !== value)].slice(0, MAX);
}

export function loadRecent(): string[] {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    return Array.isArray(raw)
      ? raw.filter((v) => typeof v === "string").slice(0, MAX)
      : [];
  } catch {
    return [];
  }
}

export function saveRecent(list: string[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(list));
  } catch {
    /* 无痕模式等情况下忽略 */
  }
}

export function loadSelectedHost(): string | null {
  try {
    return localStorage.getItem(SELECTED);
  } catch {
    return null;
  }
}

export function saveSelectedHost(id: string) {
  try {
    localStorage.setItem(SELECTED, id);
  } catch {
    /* 忽略 */
  }
}
