/*
 * 设置是一个弹窗（B180），开在当前页面上面，地址里用 ?settings=<id> 表示。
 * 旧的 /settings/<id> 地址都还能用：路由在加载前把它换成“上一个页面 + ?settings=<id>”，
 * 所以各模块里写死的设置链接、网盘授权的回调都不用改。
 */

export const SETTINGS_PARAM = "settings";

// 合并掉的标签，旧地址跳到合并后的那一页。
const aliases: Record<string, string> = {
  github: "git",
  "ai-usage": "assistant",
  errors: "maintenance",
};

// 只和一个功能有关的设置挪回了功能页。
const elsewhere: Record<string, string> = {
  cards: "/cards?tab=settings",
  "focus-block": "/calendar/focus?block=1",
};

// 最后一个不是设置的页面，弹窗开在它上面。直接打开设置地址时用今日页。
let lastPage = "/";
let lastHadDialog = false;

/** SettingsHost 每次地址变化时调用。 */
export function rememberPage(pathname: string, search: string) {
  if (pathname === "/settings" || pathname.startsWith("/settings/")) return;
  const params = new URLSearchParams(search);
  lastHadDialog = params.has(SETTINGS_PARAM);
  params.delete(SETTINGS_PARAM);
  params.delete("section");
  const rest = params.toString();
  lastPage = pathname + (rest ? `?${rest}` : "");
}

/** 弹窗已经开着时，设置里的链接换标签不再多记一条历史。 */
export function dialogIsOpen() {
  return lastHadDialog;
}

/**
 * /settings/<tab>?a=b 换成要去的地址。
 * ?section=xxx 变成 #xxx：路由加载时拿不到地址里的 #，所以链接改用 section 传。
 */
export function settingsTarget(tab: string | undefined, url: URL): string {
  if (tab && elsewhere[tab]) return elsewhere[tab];
  const id = tab ? (aliases[tab] ?? tab) : "";
  const base = new URL(lastPage, url.origin);
  const params = new URLSearchParams(base.search);
  let hash = "";
  url.searchParams.forEach((value, key) => {
    if (key === "section") hash = `#${value}`;
    else params.set(key, value);
  });
  params.set(SETTINGS_PARAM, id);
  return `${base.pathname}?${params.toString()}${hash}`;
}
