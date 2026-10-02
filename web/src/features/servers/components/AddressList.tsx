import { Copy } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import type { HostAddress } from "../api";

/** 公网的排前面，IPv4 在 IPv6 前面。 */
export function sortAddresses(list: HostAddress[]): HostAddress[] {
  return [...list].sort(
    (a, b) =>
      Number(b.public) - Number(a.public) ||
      (a.family === b.family ? 0 : a.family === "v4" ? -1 : 1),
  );
}

/**
 * 网络卡片底部的地址（B82）：IPv4、IPv6 各带复制按钮。内网的灰字。
 * bare 时不画顶部分隔线（单独一张卡片时）。
 */
export default function AddressList({
  addresses,
  bare,
}: {
  addresses: HostAddress[];
  bare?: boolean;
}) {
  const t = useT();
  if (addresses.length === 0) return null;
  const copy = (ip: string) =>
    navigator.clipboard
      ?.writeText(ip)
      .then(() => toast(t("Copied")))
      .catch(() => toast({ message: t("Could not copy"), tone: "error" }));
  return (
    <ul
      className={`servers-addresses${bare ? " bare" : ""}`}
      aria-label={t("IP addresses")}
    >
      {sortAddresses(addresses).map((a) => (
        <li key={a.ip} className={a.public ? "" : "private"}>
          <span className="servers-address-family">
            {a.family === "v4" ? "IPv4" : "IPv6"}
          </span>
          <span className="xc-mono servers-address-ip" title={a.ip}>
            {a.ip}
          </span>
          {!a.public && <small>{t("Private address")}</small>}
          <button
            type="button"
            className="xc-btn ghost small"
            title={t("Copy")}
            aria-label={`${t("Copy")} ${a.ip}`}
            onClick={() => void copy(a.ip)}
          >
            <Copy size={13} />
          </button>
        </li>
      ))}
    </ul>
  );
}
