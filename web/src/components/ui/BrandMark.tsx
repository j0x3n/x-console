/**
 * 站标（B175）：LED 像素风的 “X_”。左边 X 用主色，右边光标用辅色，底是主题色的浅色。
 * 颜色取 tokens.css 的 --xc-logo-*，跟着主题色变。浏览器和安装图标固定用赛博配色。
 */
const ROWS = [
  "##...##....",
  "###.###....",
  ".#####.....",
  "..###......",
  ".#####.....",
  "###.###.###",
  "##...##.###",
];
const CELL = 4.2;
const GAP = CELL * 0.2;
const LEFT = 32 - (ROWS[0].length * CELL) / 2;
const TOP = 32 - (ROWS.length * CELL) / 2;
const CURSOR_COLUMN = 8;
const PIXELS = ROWS.flatMap((row, y) =>
  [...row].flatMap((cell, x) =>
    cell === "#"
      ? [
          {
            x: +(LEFT + x * CELL + GAP / 2).toFixed(2),
            y: +(TOP + y * CELL + GAP / 2).toFixed(2),
            cursor: x >= CURSOR_COLUMN,
          },
        ]
      : [],
  ),
);
const SIZE = +(CELL - GAP).toFixed(2);

export default function BrandMark({ size = 32 }: { size?: number | string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 64 64"
      width={size}
      height={size}
      aria-hidden="true"
      focusable="false"
      style={{ verticalAlign: "middle", flexShrink: 0 }}
    >
      <rect width="64" height="64" rx="15" fill="var(--xc-logo-bg)" />
      {PIXELS.map((p) => (
        <rect
          key={`${p.x},${p.y}`}
          x={p.x}
          y={p.y}
          width={SIZE}
          height={SIZE}
          rx="1.2"
          fill={p.cursor ? "var(--xc-logo-fg-2)" : "var(--xc-logo-fg)"}
        />
      ))}
    </svg>
  );
}
