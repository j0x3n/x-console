// B44 一次性脚本：把样式里写死的 font-size 像素换成字号令牌。
// 令牌按桌面像素命名（--fs-12 在桌面就是 12px），手机上的值在 styles/tokens.css。
// 用法：node scripts/font-tokens.mjs        （改文件并打印统计）
//       node scripts/font-tokens.mjs --check  （只检查，有写死的像素就失败，CI 用）
import { readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const root = new URL("../src/", import.meta.url).pathname;
// 桌面像素 → 令牌名。小于 9px 的是图标里的小圆点之类，20px 以上是大标题，都保留像素。
export const TOKENS = {
  "9": "--fs-9",
  "10": "--fs-10",
  "10.5": "--fs-10-5",
  "11": "--fs-11",
  "11.5": "--fs-11-5",
  "12": "--fs-12",
  "12.5": "--fs-12-5",
  "13": "--fs-13",
  "13.5": "--fs-13-5",
  "14": "--fs-14",
  "15": "--fs-15",
  "16": "--fs-16",
  "17": "--fs-17",
  "18": "--fs-18",
  "19": "--fs-19",
};
// 令牌的定义文件和测试开关样式本身不检查。
const SKIP = new Set(["styles/tokens.css"]);

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (name.endsWith(".css")) out.push(path);
  }
  return out;
}

const check = process.argv.includes("--check");
const stats = {};
const offenders = [];
for (const file of walk(root)) {
  const rel = relative(root, file);
  if (SKIP.has(rel)) continue;
  const text = readFileSync(file, "utf8");
  const next = text.replace(
    /(font-size:\s*)([0-9]+(?:\.[0-9]+)?)px/g,
    (match, prefix, size) => {
      const token = TOKENS[size];
      if (!token) return match;
      stats[size] = (stats[size] ?? 0) + 1;
      if (check) offenders.push(`${rel}: font-size: ${size}px`);
      return `${prefix}var(${token})`;
    },
  );
  if (!check && next !== text) writeFileSync(file, next);
}

if (check) {
  if (offenders.length) {
    console.error(
      "这些地方写死了字号，改用 styles/tokens.css 里的 --fs-* 令牌（见 docs/07-design.md）：\n" +
        offenders.join("\n"),
    );
    process.exit(1);
  }
  console.log("字号都用了令牌。");
} else {
  console.log("替换统计（桌面像素：处数）：", stats);
}
