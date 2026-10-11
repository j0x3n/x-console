/* 专注屏蔽的名单：文本框里一行一项，保存前整理。服务端会再检查一遍，这里只做最简单的整理。 */

/** 网址按换行、逗号和空格分开。网址里没有空格。 */
export function parseDomains(text: string): string[] {
  return dedupe(text.split(/[\s,，;；]+/).map((s) => s.trim()));
}

/** 程序名按换行和逗号分开，名字里可以有空格。 */
export function parseApps(text: string): string[] {
  return dedupe(text.split(/[\n,，;；]+/).map((s) => s.trim()));
}

export function joinLines(items: readonly string[]): string {
  return items.join("\n");
}

function dedupe(items: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of items) {
    const key = appKey(item);
    if (!item || seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

/** 程序名比较时不分大小写，也不看 .exe。 */
export function appKey(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/\.exe$/, "");
}

/** 把建议的程序加进文本框里的名单，已有的不重复。返回新文本和新加了几个。 */
export function addApps(
  text: string,
  suggestions: readonly string[],
): { text: string; added: number } {
  const have = parseApps(text);
  const keys = new Set(have.map(appKey));
  const added: string[] = [];
  for (const name of suggestions) {
    const key = appKey(name);
    if (!key || keys.has(key)) continue;
    keys.add(key);
    added.push(name);
  }
  return { text: joinLines([...have, ...added]), added: added.length };
}

/** 两个名单是不是一样（顺序也要一样）。 */
export function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i]);
}
