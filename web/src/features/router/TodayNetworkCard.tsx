import { Link } from "react-router";
import { ArrowDownUp, Globe, MonitorSmartphone } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { formatRate } from "../servers/lib";
import { useRouterStatus } from "./api";
import "./i18n";
import "./router.css";

/** 今日页的“网络”卡片（B65）：WAN 口、在线设备数、当前速率。 */
export default function TodayNetworkCard() {
  const t = useT();
  const status = useRouterStatus();
  if (status.isPending || status.isError)
    return (
      <QueryState
        query={status}
        setupTo="/settings/router"
        setupHint={t("Connect your OpenWrt router")}
      />
    );
  const s = status.data;
  const wan = s.wan;
  const ip = wan?.ipv4[0]?.replace(/\/\d+$/, "");
  // B88：和其他卡片一样，一行一项：图标、名称和说明、右边的值
  return (
    <div className="xc-list">
      <Link className="today-row" to="/router">
        <span
          className={`today-row-icon${wan?.up ? " ok" : wan ? " danger" : ""}`}
        >
          <Globe size={15} />
        </span>
        <span className="today-row-main">
          <strong>{t("WAN")}</strong>
          <small className="xc-mono">{wan?.up && ip ? ip : "—"}</small>
        </span>
        <span className={`xc-badge ${wan ? (wan.up ? "ok" : "danger") : ""}`}>
          {wan ? t(wan.up ? "WAN up" : "WAN down") : "—"}
        </span>
      </Link>
      <Link className="today-row" to="/router">
        <span className="today-row-icon">
          <MonitorSmartphone size={15} />
        </span>
        <span className="today-row-main">
          <strong>{t("Online devices")}</strong>
        </span>
        <strong className="router-today-value">{s.clientCount}</strong>
      </Link>
      <Link className="today-row" to="/router">
        <span className="today-row-icon">
          <ArrowDownUp size={15} />
        </span>
        <span className="today-row-main">
          <strong>{t("Current speed")}</strong>
        </span>
        <strong className="router-today-value">
          {s.rxRate != null && s.txRate != null
            ? `↓ ${formatRate(s.rxRate)} ↑ ${formatRate(s.txRate)}`
            : "—"}
        </strong>
      </Link>
    </div>
  );
}
