import { useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  Power,
  RefreshCw,
  Router as RouterIcon,
  RotateCcw,
  Settings,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import {
  INTERVALS,
  intervalLabel,
  setRouterInterval,
  useRouterInterval,
  type RouterInterval,
} from "./interval";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import type { Language } from "../../types/domain";
import { toast } from "../../hooks/useToast";
import { formatBytes, formatDate, formatTime } from "../../lib/time";
import type { MetricsPoint } from "../servers/api";
import MetricChart from "../servers/components/MetricChart";
import { formatRate, formatUptime, niceRateMax } from "../servers/lib";
import {
  isNotConfigured,
  useRebootRouter,
  useRestartInterface,
  useRouterClients,
  useRouterStatus,
  useRouterTraffic,
  type RouterClient,
  type RouterInterface,
  type RouterStatus,
  type TrafficRange,
} from "./api";

type Tab = "clients" | "traffic";
const TAB_KEY = "router.tab";

function readTab(): Tab {
  try {
    return localStorage.getItem(TAB_KEY) === "traffic" ? "traffic" : "clients";
  } catch {
    return "clients";
  }
}

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 路由器（B65）：概要、接口、在线设备和 WAN 口流量。 */
export default function RouterPage() {
  const t = useT();
  const navigate = useNavigate();
  const status = useRouterStatus();
  const reboot = useRebootRouter();
  const [tab, setTabState] = useState<Tab>(readTab);
  const [range, setRange] = useState<TrafficRange>("24h");
  const setTab = (next: Tab) => {
    setTabState(next);
    try {
      localStorage.setItem(TAB_KEY, next);
    } catch {
      /* 存不了就算了 */
    }
  };
  const notConfigured = status.isError && isNotConfigured(status.error);

  const onReboot = async () => {
    const ok = await confirmAction({
      title: t("Reboot the router?"),
      description: "重启时家里会断网一两分钟。",
      confirmLabel: t("Reboot router"),
      typeToConfirm: "重启",
    });
    if (!ok) return;
    reboot.mutate(undefined, {
      onSuccess: () =>
        toast(
          s?.source === "push"
            ? t("Queued. The router runs it on its next report.")
            : t("Rebooting. It takes a minute or two."),
        ),
      onError: fail,
    });
  };

  let content;
  if (status.isPending) content = <Loading />;
  else if (notConfigured) content = <SetupGuide />;
  else if (status.isError)
    content = (
      <ErrorState error={status.error} onRetry={() => status.refetch()} />
    );
  else
    content = (
      <>
        <RouterStats status={status.data} />
        <InterfacesCard
          items={status.data.interfaces}
          queued={status.data.source === "push"}
        />
        <Toolbar
          start={
            <nav className="xc-tabs">
              <button
                className={tab === "clients" ? "active" : ""}
                onClick={() => setTab("clients")}
              >
                {t("Online devices")}
              </button>
              <button
                className={tab === "traffic" ? "active" : ""}
                onClick={() => setTab("traffic")}
              >
                {t("Traffic")}
              </button>
            </nav>
          }
          end={
            tab === "traffic" && (
              <Segmented
                label={t("Traffic")}
                value={range}
                onChange={setRange}
                options={[
                  { value: "24h", label: t("Last 24 hours") },
                  { value: "7d", label: t("Last 7 days") },
                ]}
              />
            )
          }
        />
        {tab === "clients" ? <ClientsTable /> : <TrafficView range={range} />}
      </>
    );

  const s = status.data;
  return (
    <div className="xc-page router-page">
      <PageHeading
        title={t("Router")}
        subtitle={
          s
            ? [s.hostname, s.model, s.firmware].filter(Boolean).join(" · ")
            : undefined
        }
        aside={
          !notConfigured && (
            <>
              <IntervalPicker />
              <MoreMenu
                label={t("More")}
                items={[
                  {
                    key: "settings",
                    label: t("Router settings"),
                    icon: <Settings size={14} />,
                    onSelect: () => navigate("/settings/router"),
                  },
                  {
                    key: "reboot",
                    label: t("Reboot router"),
                    icon: <Power size={14} />,
                    danger: true,
                    onSelect: onReboot,
                  },
                ]}
              />
            </>
          )
        }
      />
      {content}
    </div>
  );
}

/** B93：刷新频率。显示当前间隔，可以改到 1 秒，也可以暂停。 */
function IntervalPicker() {
  const t = useT();
  const value = useRouterInterval();
  return (
    <label className="router-interval" title={t("Refresh interval")}>
      <RefreshCw size={14} className={value ? "" : "is-paused"} />
      <select
        className="xc-select"
        aria-label={t("Refresh interval")}
        value={value}
        onChange={(e) =>
          setRouterInterval(Number(e.target.value) as RouterInterval)
        }
      >
        {INTERVALS.map((v) => (
          <option key={v} value={v}>
            {intervalLabel(v)}
          </option>
        ))}
      </select>
    </label>
  );
}

function SetupGuide() {
  const t = useT();
  return (
    <EmptyState
      title={t("Connect your OpenWrt router")}
      icon={<RouterIcon size={26} />}
    >
      <span>通过路由器自带的 ubus 接口读状态，路由器上不用装代理。</span>
      <Link className="xc-btn primary" to="/settings/router">
        {t("Set up the router")}
      </Link>
    </EmptyState>
  );
}

function RouterStats({ status }: { status: RouterStatus }) {
  const t = useT();
  const language = useLanguage();
  const wan = status.wan;
  const wanIP = wan?.ipv4[0]?.replace(/\/\d+$/, "");
  const hasRate = status.rxRate != null && status.txRate != null;
  return (
    <StatStrip label={t("Router")}>
      <StatCard
        label={t("WAN")}
        caption={wan?.proto}
        value={wan ? t(wan.up ? "WAN up" : "WAN down") : "—"}
        tone={wan ? (wan.up ? "ok" : "danger") : undefined}
        foot={wan ? (wan.up ? (wanIP ?? "") : "") : t("No WAN interface")}
      />
      <StatCard
        label={t("Online devices")}
        value={status.clientCount}
        foot={`${t("Updated at")} ${formatTime(status.checkedAt, language)}`}
      />
      <StatCard
        label={t("Current speed")}
        value={hasRate ? `↓ ${formatRate(status.rxRate!)}` : "—"}
        foot={hasRate ? `↑ ${formatRate(status.txRate!)}` : " "}
      />
      <StatCard
        label={t("Running for")}
        value={formatUptime(status.uptimeSeconds, language === "zh")}
        foot={
          wan?.up && wan.uptimeSeconds != null
            ? `WAN ${formatUptime(wan.uptimeSeconds, language === "zh")}`
            : " "
        }
      />
    </StatStrip>
  );
}

function InterfacesCard({
  items,
  queued,
}: {
  items: RouterInterface[];
  queued: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const restart = useRestartInterface();
  const onRestart = async (name: string) => {
    const ok = await confirmAction({
      title: `${t("Restart interface")} ${name}？`,
      description: name.startsWith("wan")
        ? "重启 WAN 口会断网几秒到几十秒，拨号上网时会重新拨号。"
        : "这个接口上的设备会断开几秒。",
      confirmLabel: t("Restart"),
    });
    if (!ok) return;
    restart.mutate(name, {
      onSuccess: () =>
        toast(
          queued
            ? t("Queued. The router runs it on its next report.")
            : t("Interface restarted"),
        ),
      onError: fail,
    });
  };
  return (
    <section className="xc-card router-ifaces">
      <div className="xc-card-head">
        <h2>{t("Interfaces")}</h2>
        <span className="xc-muted">{items.length}</span>
      </div>
      <ul className="router-iface-list">
        {items.map((i) => (
          <li key={i.name}>
            <span className={`xc-dot ${i.up ? "ok" : "danger"}`} />
            <div className="router-iface-main">
              <strong>{i.name}</strong>
              <small className="xc-muted">
                {[i.proto, i.device].filter(Boolean).join(" · ")}
              </small>
            </div>
            <span
              className="router-iface-ip"
              title={[...i.ipv4, ...i.ipv6].join("\n")}
            >
              {i.ipv4[0] ?? i.ipv6[0] ?? "—"}
            </span>
            <small className="router-iface-uptime xc-muted">
              {i.up && i.uptimeSeconds != null
                ? formatUptime(i.uptimeSeconds, language === "zh")
                : t("WAN down")}
            </small>
            <button
              type="button"
              className="xc-btn small"
              title={`${t("Restart interface")} ${i.name}`}
              aria-label={`${t("Restart interface")} ${i.name}`}
              disabled={restart.isPending}
              onClick={() => onRestart(i.name)}
            >
              <RotateCcw size={13} />
              <span className="router-btn-label">{t("Restart")}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ClientsTable() {
  const t = useT();
  const language = useLanguage();
  const clients = useRouterClients();
  if (clients.isPending) return <Loading />;
  if (clients.isError)
    return (
      <ErrorState error={clients.error} onRetry={() => clients.refetch()} />
    );
  const items = clients.data.items;
  const now = clients.dataUpdatedAt || Date.now();
  if (items.length === 0) return <EmptyState title={t("No devices online")} />;
  return (
    <div className="xc-card router-clients">
      <div className="xc-table-wrap">
        <table className="xc-table">
          <thead>
            <tr>
              <th>{t("Name")}</th>
              <th>IP</th>
              <th className="router-col-mac">MAC</th>
              <th>{t("Online for")}</th>
              <th className="router-col-lease">{t("Lease expires")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <ClientRow key={c.mac} c={c} language={language} now={now} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ClientRow({
  c,
  language,
  now,
}: {
  c: RouterClient;
  language: Language;
  now: number;
}) {
  return (
    <tr>
      <td className="router-client-name" title={c.name ?? c.mac}>
        {c.name ?? "—"}
      </td>
      <td className="router-mono" title={(c.ipv6 ?? []).join("\n")}>
        {c.ip ?? c.ipv6?.[0] ?? "—"}
      </td>
      <td className="router-mono router-col-mac">{c.mac}</td>
      <td className="router-num">
        {c.onlineSince
          ? formatUptime(
              Math.max(0, (now - new Date(c.onlineSince).getTime()) / 1000),
              language === "zh",
            )
          : "—"}
      </td>
      <td className="router-num router-col-lease">
        {c.leaseExpiresAt
          ? formatDate(c.leaseExpiresAt, language) +
            " " +
            formatTime(c.leaseExpiresAt, language)
          : "—"}
      </td>
    </tr>
  );
}

const S1 = "var(--servers-series-1)";
const S2 = "var(--servers-series-2)";

function TrafficView({ range }: { range: TrafficRange }) {
  const t = useT();
  const traffic = useRouterTraffic(range);
  let body;
  if (traffic.isPending) body = <Loading />;
  else if (traffic.isError)
    body = (
      <ErrorState error={traffic.error} onRetry={() => traffic.refetch()} />
    );
  else if (traffic.data.points.length === 0)
    body = (
      <EmptyState title={t("No traffic yet")}>
        <span>{t("Samples are taken once a minute.")}</span>
      </EmptyState>
    );
  else {
    // 借用服务器页的曲线图，它按 netRx、netTx 两个字段画。
    const points = traffic.data.points.map(
      (p) =>
        ({
          at: p.at,
          netRx: p.rxRate,
          netTx: p.txRate,
        }) as MetricsPoint,
    );
    body = (
      <MetricChart
        title={t("WAN traffic")}
        points={points}
        stepSeconds={traffic.data.stepSeconds}
        format={formatRate}
        nice={niceRateMax}
        series={[
          { key: "netRx", label: t("Downstream"), color: S1 },
          { key: "netTx", label: t("Upstream"), color: S2 },
        ]}
      />
    );
  }
  return (
    <div className="xc-card router-traffic">
      {body}
      {traffic.data && traffic.data.points.length > 0 && (
        <p className="router-traffic-total">
          {t("Total in this range")}：↓ {formatBytes(traffic.data.rxBytes)} ↑{" "}
          {formatBytes(traffic.data.txBytes)}
        </p>
      )}
    </div>
  );
}
