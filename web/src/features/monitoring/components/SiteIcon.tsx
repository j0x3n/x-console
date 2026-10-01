import { useState } from "react";
import { API_BASE } from "../../../api/client";
import { hostOf } from "../lib";

/**
 * 网站的站标（B50）。后端抓到了就显示，没有或加载失败时显示域名首字母。
 * 只给 id 和 iconAt 时按 http 监控取图。
 */
export default function SiteIcon({
  id,
  iconAt,
  target,
}: {
  id?: number;
  iconAt?: string;
  target: string;
}) {
  const [broken, setBroken] = useState(false);
  const host = hostOf(target).replace(/^www\./, "");
  if (id && iconAt && !broken)
    return (
      <img
        className="monitoring-site-icon"
        src={`${API_BASE}/monitors/${id}/icon?v=${encodeURIComponent(iconAt)}`}
        alt=""
        loading="lazy"
        onError={() => setBroken(true)}
      />
    );
  return (
    <span className="monitoring-site-icon letter" aria-hidden>
      {(host[0] ?? "?").toUpperCase()}
    </span>
  );
}
