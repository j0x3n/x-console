import { useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { Bell, Cable, Wifi } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { SearchBox, Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import type { Language } from "../../types/domain";
import { formatBytes, relativeTime } from "../../lib/time";
import { formatRate, formatUptime } from "../servers/lib";
import { useRouterDevices, type RouterDevice } from "./api";
import DeviceDrawer from "./DeviceDrawer";

type Show = "online" | "all";
const SHOW_KEY = "router.devices.show";

function readShow(): Show {
  try {
    return localStorage.getItem(SHOW_KEY) === "all" ? "all" : "online";
  } catch {
    return "online";
  }
}

/** 搜名字、备注、IP、MAC。 */
function matches(d: RouterDevice, q: string) {
  if (!q) return true;
  const text = [d.displayName, d.name, d.note, d.ip, d.mac]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return text.includes(q.toLowerCase());
}

/** B228：主动上报模式下的设备表。点一行打开右边的详情面板。 */
export default function DevicesView() {
  const t = useT();
  const devices = useRouterDevices();
  const [search, setSearch] = useSearchParams();
  const [show, setShowState] = useState<Show>(readShow);
  const [q, setQ] = useState("");
  const setShow = (next: Show) => {
    setShowState(next);
    try {
      localStorage.setItem(SHOW_KEY, next);
    } catch {
      /* 存不了就算了 */
    }
  };
  const openMAC = search.get("device")?.toUpperCase() ?? "";
  const open = (mac: string | null) => {
    const next = new URLSearchParams(search);
    if (mac) next.set("device", mac);
    else next.delete("device");
    setSearch(next, { replace: true });
  };

  const items = devices.data?.items;
  const list = useMemo(
    () =>
      (items ?? []).filter(
        (d) => (show === "all" || d.online) && matches(d, q.trim()),
      ),
    [items, show, q],
  );

  let body;
  if (devices.isPending) body = <Loading />;
  else if (devices.isError)
    body = (
      <ErrorState error={devices.error} onRetry={() => devices.refetch()} />
    );
  else if (list.length === 0)
    body = (
      <EmptyState
        title={q ? t("No matching devices") : t("No devices online")}
      />
    );
  else
    body = (
      <DeviceTable
        items={list}
        traffic={devices.data.trafficAvailable}
        onOpen={open}
      />
    );

  const current = items?.find((d) => d.mac === openMAC);
  return (
    <>
      <Toolbar
        className="router-dev-toolbar"
        start={
          <Segmented
            label={t("Devices")}
            value={show}
            onChange={setShow}
            options={[
              { value: "online", label: t("Online") },
              { value: "all", label: t("All") },
            ]}
          />
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search name, note, IP or MAC")}
          />
        }
      />
      {devices.data && !devices.data.trafficAvailable && (
        <p className="router-hint">
          在路由器上装
          nlbwmon（带宽监控）后显示每台设备的流量。装好后重新生成令牌，再执行一次安装命令。
        </p>
      )}
      {body}
      {current && (
        <DeviceDrawer
          device={current}
          traffic={devices.data?.trafficAvailable ?? false}
          onClose={() => open(null)}
        />
      )}
    </>
  );
}

function DeviceTable({
  items,
  traffic,
  onOpen,
}: {
  items: RouterDevice[];
  traffic: boolean;
  onOpen: (mac: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  return (
    <div className="xc-card router-clients">
      <div className="xc-table-wrap">
        <table className="xc-table router-dev-table">
          <thead>
            <tr>
              <th>{t("Name")}</th>
              <th className="router-col-conn" aria-label={t("Connection")} />
              <th className="router-col-ip">IP</th>
              {traffic && (
                <>
                  <th className="router-num router-col-rx">↓</th>
                  <th className="router-num router-col-tx">↑</th>
                  <th className="router-num router-col-today">{t("Today")}</th>
                </>
              )}
              <th className="router-col-since">{t("Online for")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((d) => (
              <DeviceRow
                key={d.mac}
                d={d}
                traffic={traffic}
                language={language}
                onOpen={() => onOpen(d.mac)}
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ConnIcon({ conn }: { conn: RouterDevice["conn"] }) {
  const t = useT();
  if (conn === "wifi")
    return (
      <span
        className="router-dev-conn"
        title={t("Wi-Fi")}
        aria-label={t("Wi-Fi")}
      >
        <Wifi size={14} />
      </span>
    );
  if (conn === "wired")
    return (
      <span
        className="router-dev-conn"
        title={t("Wired")}
        aria-label={t("Wired")}
      >
        <Cable size={14} />
      </span>
    );
  return null;
}

/** 在线的写在线多久，离线的写多久前离线。 */
export function sinceText(d: RouterDevice, language: Language, now: number) {
  if (d.online)
    return d.onlineSince
      ? formatUptime(
          Math.max(0, (now - new Date(d.onlineSince).getTime()) / 1000),
          language === "zh",
        )
      : "—";
  const ago = relativeTime(d.lastSeen, language);
  return language === "zh" ? `${ago}离线` : `offline ${ago}`;
}

/** 速率是 0 也显示“0”，灰色；没有数据才是“—”。 */
function Rate({ value }: { value: number | null | undefined }) {
  if (value == null) return <>—</>;
  if (value < 1) return <span className="router-zero">0</span>;
  return <>{formatRate(value)}</>;
}

/** 今天的下载加上传。 */
export function todayBytes(d: RouterDevice) {
  if (d.todayRx == null && d.todayTx == null) return null;
  return (d.todayRx ?? 0) + (d.todayTx ?? 0);
}

function DeviceRow({
  d,
  traffic,
  language,
  onOpen,
}: {
  d: RouterDevice;
  traffic: boolean;
  language: Language;
  onOpen: () => void;
}) {
  const today = todayBytes(d);
  const t = useT();
  const hostname = d.note && d.name ? d.name : null;
  return (
    <tr
      className={`router-dev-row${d.online ? "" : " is-offline"}`}
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter") onOpen();
      }}
    >
      <td className="router-dev-name" title={d.mac}>
        <span className={`xc-dot ${d.online ? "ok" : ""}`} />
        <span className="router-dev-label">
          <strong className={d.note ? "has-note" : ""}>{d.displayName}</strong>
          {hostname && <small>{hostname}</small>}
          {d.notify && (
            <Bell
              size={12}
              className="router-dev-bell"
              aria-label={t("Notifications on")}
            />
          )}
        </span>
        <small className="router-dev-sub">
          {[
            d.ip,
            d.conn === "wifi"
              ? "Wi-Fi"
              : d.conn === "wired"
                ? t("Wired")
                : null,
          ]
            .filter(Boolean)
            .join(" · ")}
        </small>
      </td>
      <td className="router-col-conn">
        <ConnIcon conn={d.conn} />
      </td>
      <td
        className="router-mono router-col-ip"
        title={(d.ipv6 ?? []).join("\n")}
      >
        {d.ip ?? d.ipv6?.[0] ?? "—"}
      </td>
      {traffic && (
        <>
          <td className="router-num router-col-rx">
            <Rate value={d.online ? d.rxRate : null} />
          </td>
          <td className="router-num router-col-tx">
            <Rate value={d.online ? d.txRate : null} />
          </td>
          <td className="router-num router-col-today">
            {today == null ? "—" : formatBytes(today)}
          </td>
        </>
      )}
      <td className="router-num router-col-since">
        {sinceText(d, language, Date.now())}
      </td>
    </tr>
  );
}
