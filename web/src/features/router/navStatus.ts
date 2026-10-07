import type { NavStatus } from "../../lib/navBadges";
import { formatRate } from "../servers/lib";
import { useRouterStatus } from "./api";

/** 左栏用的短速率：512K、3.2M、1.1G（每秒字节）。 */
export function shortRate(bytes: number): string {
  if (bytes < 1024) return `${Math.round(bytes)}B`;
  const units = ["K", "M", "G"];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 ? Math.round(v) : v.toFixed(1)}${units[i]}`;
}

/**
 * B93：左栏“路由器”右边：WAN 在线状态点、在线设备数、上传下载里较快的那个。
 * 刷新频率和路由器页的设置一样。没配置路由器时不显示。
 */
export function useRouterNavStatus(): NavStatus | null {
  const status = useRouterStatus();
  const s = status.data;
  if (!s) return null;
  const wan = s.wan;
  const rx = s.rxRate ?? null;
  const tx = s.txRate ?? null;
  const fastest = rx == null && tx == null ? null : Math.max(rx ?? 0, tx ?? 0);
  const parts = [`${s.clientCount}`];
  if (fastest != null) parts.push(shortRate(fastest));
  const title = [
    wan ? (wan.up ? "WAN 在线" : "WAN 断开") : "",
    `${s.clientCount} 台设备`,
    rx != null ? `下载 ${formatRate(rx)}` : "",
    tx != null ? `上传 ${formatRate(tx)}` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  return {
    dot: wan ? (wan.up ? "ok" : "danger") : undefined,
    text: parts.join(" · "),
    title,
  };
}
