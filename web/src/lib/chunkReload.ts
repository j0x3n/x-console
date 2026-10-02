/*
 * 部署后浏览器里还开着旧页面时，按需加载的旧文件已经不在服务器上了，
 * 打开新页面会报“加载失败”。遇到这种错误自动刷新一次，拿新版本。
 * 30 秒内只刷新一次，避免网络断了时一直刷新。
 */

const KEY = "xc.chunkReloadAt";
const WINDOW_MS = 30_000;

const patterns = [
  /Loading chunk \d+ failed/i,
  /Failed to fetch dynamically imported module/i,
  /error loading dynamically imported module/i,
  /Importing a module script failed/i,
  /Unable to preload CSS/i,
  /^Load failed$/i,
];

export function isChunkLoadError(error: unknown): boolean {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === "string"
        ? error
        : "";
  return patterns.some((p) => p.test(message.trim()));
}

/** 刷新一次页面。30 秒内已经刷新过就不再刷新，返回 false。 */
export function reloadOnce(): boolean {
  try {
    const last = Number(sessionStorage.getItem(KEY) ?? 0);
    if (Date.now() - last < WINDOW_MS) return false;
    sessionStorage.setItem(KEY, String(Date.now()));
  } catch {
    return false;
  }
  location.reload();
  return true;
}
