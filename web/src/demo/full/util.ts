/** 相对现在的时间。min 可以是负数（过去）。 */
export const at = (min: number) =>
  new Date(Date.now() + min * 60_000).toISOString();

/** 相对今天的某一天某个时刻，本地时间。 */
export function day(offset: number, hm = "09:00") {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  const [h, m] = hm.split(":").map(Number);
  d.setHours(h, m, 0, 0);
  return d.toISOString();
}

/** 相对今天的日期，YYYY-MM-DD，本地时间。 */
export function date(offset: number) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 固定种子的伪随机数，每次刷新数据一样。 */
export function rand(seed: number) {
  let s = seed;
  return () => {
    s = (s * 16807) % 2147483647;
    return (s - 1) / 2147483646;
  };
}

export const MB = 1024 * 1024;
export const GB = 1024 * MB;
