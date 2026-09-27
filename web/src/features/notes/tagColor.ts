/** 标签可选的颜色，和项目标签用同一组。 */
export const TAG_COLORS = [
  "#cc7752",
  "#e8b454",
  "#5cc98b",
  "#4fb3a9",
  "#70b5f7",
  "#8f86f0",
  "#d57ab4",
  "#85858e",
];

/** 用户设过就用设的颜色，没设过按标签名固定选一个，同一个标签每次都一样。 */
export function tagColor(tag: string, color?: string | null): string {
  if (color) return color;
  let h = 0;
  for (const ch of tag) h = (h * 31 + ch.codePointAt(0)!) >>> 0;
  return TAG_COLORS[h % TAG_COLORS.length];
}
