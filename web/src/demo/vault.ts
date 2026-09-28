import { demoFull } from "./mode";
import { fail, json, noContent, route } from "./router";

/*
 * 隐藏内容：密码存在 localStorage，解锁状态存在 sessionStorage，
 * 刷新页面和真的一样（关掉标签页再打开会锁上）。
 */
const KEY = "xc.demo.vault";
function load() {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    return {
      configured: !!saved.password,
      password: String(saved.password ?? ""),
      unlocked: sessionStorage.getItem(KEY) === "1",
      until: new Date(Date.now() + 15 * 60_000).toISOString(),
    };
  } catch {
    return { configured: false, password: "", unlocked: false, until: "" };
  }
}
export const vault = load();
function persist() {
  try {
    localStorage.setItem(KEY, JSON.stringify({ password: vault.password }));
    if (vault.unlocked) sessionStorage.setItem(KEY, "1");
    else sessionStorage.removeItem(KEY);
  } catch {
    /* 存不了就只在内存里 */
  }
}

const status = () => ({
  configured: vault.configured,
  unlocked: vault.unlocked,
  unlockedUntil: vault.unlocked ? vault.until : undefined,
});

const unlock = () => {
  vault.unlocked = true;
  vault.until = new Date(Date.now() + 15 * 60_000).toISOString();
  persist();
};

route("GET", "/vault/status", () => (demoFull ? json(status()) : undefined));
route("POST", "/vault/setup", ({ body }) => {
  if (!demoFull) return undefined;
  if (vault.configured) return fail(409, "conflict", "已经设置过隐藏密码");
  vault.configured = true;
  vault.password = body?.password ?? "";
  unlock();
  return json(status());
});
route("POST", "/vault/unlock", ({ body }) => {
  if (!demoFull) return undefined;
  if (body?.password !== vault.password)
    return fail(401, "invalid_password", "密码不对");
  unlock();
  return json(status());
});
route("POST", "/vault/lock", () => {
  if (!demoFull) return undefined;
  vault.unlocked = false;
  persist();
  return noContent();
});
route("POST", "/vault/password", ({ body }) => {
  if (!demoFull) return undefined;
  if (body?.oldPassword !== vault.password)
    return fail(401, "invalid_password", "旧密码不对");
  vault.password = body.newPassword;
  persist();
  return noContent();
});
