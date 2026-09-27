/** 入口：2 秒内连续点 3 次 Logo。 */
export const TAP_COUNT = 3;
export const TAP_WINDOW_MS = 2000;
/** 15 分钟没有操作就自动锁定。 */
export const IDLE_LOCK_MS = 15 * 60 * 1000;
export const MIN_VAULT_PASSWORD = 6;

/**
 * 记一次点击，返回新的点击时间列表，以及这次是否凑够了次数。
 * 凑够后清空，下一轮重新数。
 */
export function registerTap(
  taps: number[],
  now: number,
): { taps: number[]; fire: boolean } {
  const recent = [...taps.filter((t) => now - t < TAP_WINDOW_MS), now];
  if (recent.length >= TAP_COUNT) return { taps: [], fire: true };
  return { taps: recent, fire: false };
}

/*
 * 关闭页面后重新打开要自动锁定。解锁时在 sessionStorage 记一笔，
 * 它在刷新后还在，关掉标签页就没了。打开页面时服务端说已解锁、
 * 但这里没有记录，说明是重新打开的，要锁上。
 */
const SESSION_KEY = "xc.vault.session";

export function markVaultSession(on: boolean) {
  try {
    if (on) sessionStorage.setItem(SESSION_KEY, "1");
    else sessionStorage.removeItem(SESSION_KEY);
  } catch {
    /* 存不了就当没有记录，重新打开时会锁上 */
  }
}

export function hasVaultSession(): boolean {
  try {
    return sessionStorage.getItem(SESSION_KEY) === "1";
  } catch {
    return false;
  }
}

/** 设置或修改隐藏密码时的检查。没问题返回空字符串。 */
export function vaultPasswordError(password: string, confirm: string): string {
  if (password.length < MIN_VAULT_PASSWORD)
    return `密码至少 ${MIN_VAULT_PASSWORD} 位`;
  if (password !== confirm) return "两次输入的密码不一致";
  return "";
}

/** Logo 被点时由侧边栏发出这个事件，隐藏内容模块监听它。 */
export const BRAND_TAP_EVENT = "xc:brand-tap";
