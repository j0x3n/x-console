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

type SortKey = "rx" | "name" | "ip" | "today";
const SORT_KEY = "router.devices.sort";

function readSort(): SortKey {
  try {
    const v = localStorage.getItem(SORT_KEY);
    return v === "name" || v === "ip" || v === "today" ? v : "rx";
  } catch {
    return "rx";
  }
}

/** IPv4 转成数字好比较，没有 IP 的排最后。 */
function ipNumber(ip: string | null | undefined) {
  const parts = ip?.split(".").map(Number);
  if (!parts || parts.length !== 4 || parts.some(Number.isNaN))
    return Number.MAX_SAFE_INTEGER;
  return parts.reduce((n, p) => n * 256 + p, 0);
}

/** B231：在线的永远排前面，再按选的列排。速率和流量从大到小。 */
function sortDevices(list: RouterDevice[], key: SortKey) {
  const byName = (a: RouterDevice, b: RouterDevice) =>
    a.displayName.localeCompare(b.displayName, "zh-CN");
  return [...list].sort((a, b) => {
    if (a.online !== b.online) return a.online ? -1 : 1;
    let diff = 0;
    if (key === "rx") diff = (b.rxRate ?? 0) - (a.rxRate ?? 0);
    else if (key === "today")
      diff = (todayBytes(b) ?? 0) - (todayBytes(a) ?? 0);
    else if (key === "ip") diff = ipNumber(a.ip) - ipNumber(b.ip);
    return diff || byName(a, b);
  });
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
export default function DevicesView({ tabs }: { tabs: React.ReactNode }) {
  const t = useT();
  const devices = useRouterDevices();
  const [search, setSearch] = useSearchParams();
  const [show, setShowState] = useState<Show>(readShow);
  const [q, setQ] = useState("");
  const [sort, setSortState] = useState<SortKey>(readSort);
  const setSort = (next: SortKey) => {
    setSortState(next);
    try {
      localStorage.setItem(SORT_KEY, next);
    } catch {
      /* 存不了就算了 */
    }
  };
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
  const traffic = devices.data?.trafficAvailable ?? false;
  // 没有设备流量时按速率排没有意义，改按名字
  const sortKey: SortKey =
    !traffic && (sort === "rx" || sort === "today") ? "name" : sort;
  const list = useMemo(
    () =>
      sortDevices(
        (items ?? []).filter(
          (d) => (show === "all" || d.online) && matches(d, q.trim()),
        ),
        sortKey,
      ),
    [items, show, q, sortKey],
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
        traffic={traffic}
        sort={sortKey}
        onSort={setSort}
        onOpen={open}
      />
    );

  const current = items?.find((d) => d.mac === openMAC);
  return (
    <>
      <Toolbar
        start={tabs}
        end={
          <>
            <Segmented
              label={t("Devices")}
              value={show}
              onChange={setShow}
              options={[
                { value: "online", label: t("Online") },
                { value: "all", label: t("All") },
              ]}
            />
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search name, note, IP or MAC")}
            />
          </>
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
  sort,
  onSort,
  onOpen,
}: {
  items: RouterDevice[];
  traffic: boolean;
  sort: SortKey;
  onSort: (key: SortKey) => void;
  onOpen: (mac: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const head = (key: SortKey, label: string, className = "") => (
    <th
      className={className}
      aria-sort={
        sort === key
          ? key === "name" || key === "ip"
            ? "ascending"
            : "descending"
          : undefined
      }
    >
      <button
        type="button"
        className={`router-sort${sort === key ? " on" : ""}`}
        onClick={() => onSort(key)}
      >
        {label}
      </button>
    </th>
  );
  return (
    <div className="xc-card router-clients">
      <div className="xc-table-wrap">
        <table className="xc-table router-dev-table">
          <thead>
            <tr>
              {head("name", t("Name"))}
              <th className="router-col-conn" aria-label={t("Connection")} />
              {head("ip", "IP", "router-col-ip")}
              {traffic && (
                <>
                  {head("rx", `↓ ${t("Download")}`, "router-num router-col-rx")}
                  <th className="router-num router-col-tx">↑ {t("Upload")}</th>
                  {head("today", t("Today"), "router-num router-col-today")}
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
            traffic && today != null
              ? `${t("Today")} ${formatBytes(today)}`
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
