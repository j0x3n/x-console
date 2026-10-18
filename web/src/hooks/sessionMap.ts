/*
 * 切换页面后保留的小块状态（B80、B249）共用的存取：
 * 放在内存的 Map 里，同时写 sessionStorage，刷新页面也在，关掉标签页就忘掉。
 */

export interface SessionMap<T> {
  get(key: string): T | undefined;
  has(key: string): boolean;
  set(key: string, value: T): void;
  delete(key: string): void;
  keys(): IterableIterator<string>;
  readonly size: number;
}

/** 从 sessionStorage 读一份。读不出来时是空的。 */
export function loadMap(storageKey: string): Map<string, unknown> {
  try {
    const raw = sessionStorage.getItem(storageKey);
    const obj = raw ? (JSON.parse(raw) as Record<string, unknown>) : {};
    return new Map(Object.entries(obj));
  } catch {
    return new Map();
  }
}

export function persistMap(storageKey: string, map: Map<string, unknown>) {
  try {
    sessionStorage.setItem(storageKey, JSON.stringify(Object.fromEntries(map)));
  } catch {
    /* 存不了就只在内存里 */
  }
}

/**
 * 一个按 key 存的表。写入后稍等一下再存 sessionStorage，关页面前补存一次。
 * 最多 max 条，多了丢掉最早写的。
 */
export function sessionMap<T>(
  storageKey: string,
  { max = 300, delay = 300 }: { max?: number; delay?: number } = {},
): SessionMap<T> {
  const map = loadMap(storageKey) as Map<string, T>;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const flush = () => {
    clearTimeout(timer);
    timer = undefined;
    persistMap(storageKey, map);
  };
  const schedule = () => {
    clearTimeout(timer);
    timer = setTimeout(flush, delay);
  };
  if (typeof window !== "undefined")
    window.addEventListener("pagehide", () => timer !== undefined && flush());
  return {
    get: (key) => map.get(key),
    has: (key) => map.has(key),
    set(key, value) {
      map.delete(key);
      map.set(key, value);
      while (map.size > max) {
        const oldest = map.keys().next().value;
        if (oldest === undefined) break;
        map.delete(oldest);
      }
      schedule();
    },
    delete(key) {
      if (map.delete(key)) schedule();
    },
    keys: () => map.keys(),
    get size() {
      return map.size;
    },
  };
}
