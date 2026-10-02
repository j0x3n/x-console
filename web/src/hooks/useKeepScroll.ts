import { useEffect, type RefObject } from "react";

/*
 * 切换页面后保留位置（B80）。不把整页留在内存里，只记两样：
 * - 每个模块最后看的地址，点左栏一级菜单时回到那里；
 * - 滚动位置，按 key 记。页面主体按完整地址记，页面里自己滚动的区域用 useKeepScroll。
 * 都存在内存里，同时写 sessionStorage，刷新页面也能恢复，关掉标签页就忘掉。
 */

const SCROLL_KEY = "xc.scroll";
const PATH_KEY = "xc.lastPath";
/** 回来时内容还没加载出来，最多等这么久。 */
const RESTORE_WAIT_MS = 2000;
/** 最多记这么多个位置，多了丢掉最早的。 */
const MAX_ENTRIES = 300;

function load(key: string): Map<string, unknown> {
  try {
    const raw = sessionStorage.getItem(key);
    const obj = raw ? (JSON.parse(raw) as Record<string, unknown>) : {};
    return new Map(Object.entries(obj));
  } catch {
    return new Map();
  }
}

function persist(key: string, map: Map<string, unknown>) {
  try {
    sessionStorage.setItem(key, JSON.stringify(Object.fromEntries(map)));
  } catch {
    /* 存不了就只在内存里 */
  }
}

const scrolls = load(SCROLL_KEY) as Map<string, number>;
const paths = load(PATH_KEY) as Map<string, string>;
let persistTimer: ReturnType<typeof setTimeout> | undefined;

export function readScroll(key: string): number | undefined {
  const v = scrolls.get(key);
  return typeof v === "number" ? v : undefined;
}

export function saveScroll(key: string, top: number) {
  scrolls.delete(key);
  scrolls.set(key, Math.max(0, Math.round(top)));
  while (scrolls.size > MAX_ENTRIES) {
    const oldest = scrolls.keys().next().value;
    if (oldest === undefined) break;
    scrolls.delete(oldest);
  }
  clearTimeout(persistTimer);
  persistTimer = setTimeout(() => persist(SCROLL_KEY, scrolls), 300);
}

/** 地址属于哪个模块：第一段路径，比如 /notes/12 → /notes。首页和设置不记。 */
export function moduleRoot(pathname: string): string | null {
  const seg = pathname.split("/")[1];
  if (!seg || seg === "settings") return null;
  return `/${seg}`;
}

/** 记下这个模块最后看的地址（路径加查询参数）。 */
export function rememberPath(pathname: string, search: string) {
  const root = moduleRoot(pathname);
  if (!root) return;
  paths.set(root, pathname + search);
  persist(PATH_KEY, paths);
}

/**
 * 左栏一级菜单的链接（B80）：回到这个模块上次停留的地址。
 * 已经在这个模块里时回到模块首页（像 X 点“首页”回顶部）。
 */
export function lastPathFor(root: string, currentPath: string): string {
  if (moduleRoot(currentPath) === root) return root;
  return paths.get(root) ?? root;
}

type Target = HTMLElement | null | undefined;

/**
 * 记住一个滚动区域的位置，回来时恢复。
 * 等内容渲染出来再滚：内容够长了就滚过去，最多等 2 秒。用户自己先滚了就不再恢复。
 * fallbackTop：没有记录时滚到哪里（页面主体用 0，回到顶部）。
 */
export function useKeepScroll(
  ref: RefObject<HTMLElement | null> | (() => Target),
  key: string,
  options: { axis?: "x" | "y"; fallbackTop?: number } = {},
) {
  const { axis = "y", fallbackTop } = options;
  useEffect(() => {
    const el = typeof ref === "function" ? ref() : ref.current;
    if (!el || !key) return;
    const isPage = el === document.scrollingElement;
    const target = readScroll(key) ?? fallbackTop;
    let touched = false;
    let restoring = target != null;
    let frame = 0;
    const started = performance.now();
    const pos = () => (axis === "y" ? el.scrollTop : el.scrollLeft);
    const max = () =>
      axis === "y"
        ? el.scrollHeight - el.clientHeight
        : el.scrollWidth - el.clientWidth;
    const scrollTo = (v: number) => {
      if (axis === "y") el.scrollTop = v;
      else el.scrollLeft = v;
    };
    const tick = () => {
      if (touched || target == null) {
        restoring = false;
        return;
      }
      if (
        max() >= target - 1 ||
        performance.now() - started > RESTORE_WAIT_MS
      ) {
        scrollTo(Math.min(target, Math.max(0, max())));
        restoring = false;
        return;
      }
      frame = requestAnimationFrame(tick);
    };
    tick();
    // 用户自己动了就不再恢复
    const stop = () => {
      touched = true;
    };
    const onScroll = () => {
      if (restoring && !touched) return;
      saveScroll(key, pos());
    };
    const scrollTarget: HTMLElement | Window = isPage ? window : el;
    const inputTarget: HTMLElement | Document = isPage ? document : el;
    scrollTarget.addEventListener("scroll", onScroll, { passive: true });
    for (const name of ["wheel", "touchstart", "pointerdown", "keydown"])
      inputTarget.addEventListener(name, stop, { passive: true });
    return () => {
      // 不在这里记位置：卸载时元素可能已经离开页面，读到的是 0
      cancelAnimationFrame(frame);
      scrollTarget.removeEventListener("scroll", onScroll);
      for (const name of ["wheel", "touchstart", "pointerdown", "keydown"])
        inputTarget.removeEventListener(name, stop);
    };
    // ref 是固定的，按 key 重新挂
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, axis]);
}
