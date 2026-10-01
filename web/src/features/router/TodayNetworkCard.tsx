import { useT } from "../../contexts/LanguageContext";
import { QueryState } from "../overview/components/shared";
import { formatRate } from "../servers/lib";
import { useRouterStatus } from "./api";
import "./i18n";
import "./router.css";

/** 今日页的“网络”卡片（B65）：WAN 口、在线设备数、当前速率。 */
export default function TodayNetworkCard() {
  const t = useT();
  const status = useRouterStatus(60_000);
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
  return (
    <ul className="router-today">
      <li>
        <span>{t("WAN")}</span>
        <strong className={wan ? (wan.up ? "ok" : "danger") : ""}>
          {wan ? t(wan.up ? "WAN up" : "WAN down") : "—"}
        </strong>
        {wan?.up && ip && <small>{ip}</small>}
      </li>
      <li>
        <span>{t("Online devices")}</span>
        <strong>{s.clientCount}</strong>
      </li>
      <li>
        <span>{t("Current speed")}</span>
        <strong>
          {s.rxRate != null && s.txRate != null
            ? `↓ ${formatRate(s.rxRate)} ↑ ${formatRate(s.txRate)}`
            : "—"}
        </strong>
      </li>
    </ul>
  );
}
