import { fail, json, noContent, now, route, ago } from "./router";

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

route("GET", "/vault/status", () => json(status()));
route("POST", "/vault/setup", ({ body }) => {
  if (vault.configured) return fail(409, "conflict", "已经设置过隐藏密码");
  vault.configured = true;
  vault.password = body?.password ?? "";
  unlock();
  return json(status());
});
route("POST", "/vault/unlock", ({ body }) => {
  if (body?.password !== vault.password)
    return fail(401, "invalid_password", "密码不对");
  unlock();
  return json(status());
});
route("POST", "/vault/lock", () => {
  vault.unlocked = false;
  persist();
  return noContent();
});
route("POST", "/vault/password", ({ body }) => {
  if (body?.oldPassword !== vault.password)
    return fail(401, "invalid_password", "旧密码不对");
  vault.password = body.newPassword;
  persist();
  return noContent();
});

/* 隐藏笔记：id 从 900001 开始，只在解锁后出现。普通笔记照常走真实服务端。 */
interface HiddenNote {
  id: number;
  title: string;
  body: string;
  pinned: boolean;
  tags: string[];
  createdAt: string;
  updatedAt: string;
}

let nextNoteId = 900003;
const hiddenNotes: HiddenNote[] = [
  {
    id: 900001,
    title: "银行卡和保险",
    body: "- 招行储蓄卡：尾号 1234\n- 医保卡：放在书房抽屉\n- 车险到期：明年 3 月 12 日",
    pinned: true,
    tags: ["私人"],
    createdAt: ago(60 * 24 * 12),
    updatedAt: ago(60 * 5),
  },
  {
    id: 900002,
    title: "体检结果",
    body: "血脂偏高，三个月后复查。\n\n少吃油炸，每周跑步三次。",
    pinned: false,
    tags: ["健康"],
    createdAt: ago(60 * 24 * 30),
    updatedAt: ago(60 * 24 * 2),
  },
];

const summary = (n: HiddenNote) => ({
  id: n.id,
  title: n.title,
  excerpt: n.body.replace(/[#*>`-]/g, "").slice(0, 120),
  pinned: n.pinned,
  tags: n.tags,
  hidden: true,
  createdAt: n.createdAt,
  updatedAt: n.updatedAt,
});
const full = (n: HiddenNote) => ({ ...n, hidden: true });
const isDemoNote = (id: string) => Number(id) >= 900000;

route("GET", "/notes", ({ query }) => {
  if (query.get("hidden") !== "true") return undefined;
  if (!vault.unlocked) return json({ items: [] });
  const q = (query.get("q") ?? "").toLowerCase();
  const items = hiddenNotes
    .filter((n) => !q || `${n.title} ${n.body}`.toLowerCase().includes(q))
    .sort(
      (a, b) =>
        Number(b.pinned) - Number(a.pinned) ||
        b.updatedAt.localeCompare(a.updatedAt),
    )
    .map(summary);
  return json({ items });
});
route("POST", "/notes", ({ body }) => {
  if (!body?.hidden) return undefined;
  if (!vault.unlocked) return fail(403, "vault_locked", "先解锁隐藏内容");
  const n: HiddenNote = {
    id: nextNoteId++,
    title: body.title ?? "",
    body: body.body ?? "",
    pinned: !!body.pinned,
    tags: body.tags ?? [],
    createdAt: now(),
    updatedAt: now(),
  };
  hiddenNotes.push(n);
  return json(full(n), 201);
});
route("GET", "/notes/:id", ({ params }) => {
  if (!isDemoNote(params.id)) return undefined;
  const n = hiddenNotes.find((x) => x.id === Number(params.id));
  return n && vault.unlocked
    ? json(full(n))
    : fail(404, "not_found", "资源不存在");
});
route("PATCH", "/notes/:id", ({ params, body }) => {
  if (!isDemoNote(params.id)) return undefined;
  const n = hiddenNotes.find((x) => x.id === Number(params.id));
  if (!n || !vault.unlocked) return fail(404, "not_found", "资源不存在");
  for (const k of ["title", "body", "pinned", "tags"] as const)
    if (body?.[k] !== undefined)
      (n as unknown as Record<string, unknown>)[k] = body[k];
  n.updatedAt = now();
  if (body?.hidden === false) {
    // 演示里取消隐藏就当作删掉，普通笔记在真实服务端。
    hiddenNotes.splice(hiddenNotes.indexOf(n), 1);
    return json({ ...full(n), hidden: false });
  }
  return json(full(n));
});
route("DELETE", "/notes/:id", ({ params }) => {
  if (!isDemoNote(params.id)) return undefined;
  const i = hiddenNotes.findIndex((x) => x.id === Number(params.id));
  if (i >= 0) hiddenNotes.splice(i, 1);
  return noContent();
});
