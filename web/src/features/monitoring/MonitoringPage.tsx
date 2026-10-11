import { lazy, Suspense, useState } from "react";
import { useParams } from "react-router";
import { Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import {
  useMonitors,
  useScripts,
  useSubscriptions,
  useSubscriptionSummary,
} from "./api";
import CertsTab from "./components/CertsTab";
import { useParam } from "./components/common";
import {
  formatMoney,
  SPEND_CURRENCIES,
  spendView,
  type SpendCurrency,
} from "./lib";
import ScriptsTab from "./components/ScriptsTab";
import SitesTab from "./components/SitesTab";
import SubscriptionsTab from "./components/SubscriptionsTab";

// AI 额度（B152）放在监控下面，页面自带标题和统计条
const QuotasPage = lazy(() => import("../quotas/QuotasPage"));

const tabs = [
  { id: "", label: "Websites", to: "/monitoring", add: "New website" },
  {
    id: "certs",
    label: "Certificates & domains",
    to: "/monitoring/certs",
    add: "New monitor",
  },
  {
    id: "scripts",
    label: "Scripts",
    to: "/monitoring/scripts",
    add: "New script",
  },
  {
    id: "subscriptions",
    label: "Subscriptions",
    to: "/monitoring/subscriptions",
    add: "New subscription",
  },
];

export default function MonitoringPage() {
  const t = useT();
  const { tab = "" } = useParams();
  const [, setCreating] = useParam("new");
  const current = tabs.find((x) => x.id === tab) ?? tabs[0];
  const monitors = useMonitors();
  const down = (monitors.data ?? []).filter(
    (m) => m.enabled && m.lastStatus === "down",
  );
  if (tab === "quotas")
    return (
      <Suspense fallback={<Loading />}>
        <QuotasPage />
      </Suspense>
    );
  return (
    <div className="xc-page">
      {/* 页签在左栏二级菜单里（2026-10-05），这里只显示当前视图的名字 */}
      <PageHeading
        title={t(current.label)}
        subtitle={
          down.length > 0
            ? `${down.length} ${t("monitors are down")}`
            : undefined
        }
        aside={
          <button className="xc-btn primary" onClick={() => setCreating("1")}>
            <Plus size={15} /> {t(current.add)}
          </button>
        }
      />
      <MonitoringStats />
      {current.id === "" && <SitesTab />}
      {current.id === "certs" && <CertsTab />}
      {current.id === "scripts" && <ScriptsTab />}
      {current.id === "subscriptions" && <SubscriptionsTab />}
    </div>
  );
}

function MonitoringStats() {
  const t = useT();
  const monitors = useMonitors().data ?? [];
  const scripts = useScripts().data ?? [];
  const subs = useSubscriptions(false).data ?? [];
  const summary = useSubscriptionSummary().data;
  const sites = monitors.filter((m) => m.kind === "http" && m.enabled);
  const up = sites.filter((m) => m.lastStatus === "up").length;
  const down = sites.filter((m) => m.lastStatus === "down").length;
  const expiring = monitors
    .filter((m) => m.kind !== "http" && m.enabled && m.daysLeft != null)
    .sort((a, b) => (a.daysLeft ?? 0) - (b.daysLeft ?? 0));
  const soonest = expiring[0];
  const nextSub = [...subs]
    .filter((x) => x.daysLeft != null)
    .sort((a, b) => (a.daysLeft ?? 0) - (b.daysLeft ?? 0))[0];
  const [currency, setCurrency] = useState<SpendCurrency>(readCurrency);
  const spend = spendView(summary, currency);
  const switchCurrency = (e: React.SyntheticEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const next =
      SPEND_CURRENCIES[
        (SPEND_CURRENCIES.indexOf(currency) + 1) % SPEND_CURRENCIES.length
      ];
    setCurrency(next);
    try {
      localStorage.setItem(CURRENCY_KEY, next);
    } catch {
      // 存不了就只在这次有效
    }
  };
  return (
    <StatStrip label={t("Monitoring")}>
      <StatCard
        label={t("Websites")}
        caption={sites.length ? `${up}/${sites.length}` : undefined}
        value={sites.length ? up : "–"}
        unit={sites.length ? t("sites up") : undefined}
        tone={down ? "danger" : sites.length ? "ok" : undefined}
        foot={
          down
            ? `${down} ${t("monitors are down")}`
            : sites.length
              ? t("All websites up")
              : t("No websites yet")
        }
      >
        {sites.length > 0 && (
          <Segments
            parts={[
              { value: up, tone: "ok" },
              { value: down, tone: "danger" },
              { value: sites.length - up - down, tone: "muted" },
            ]}
          />
        )}
      </StatCard>
      <StatCard
        label={t("Certificates & domains")}
        value={soonest ? soonest.daysLeft : "–"}
        unit={soonest ? t("days") : undefined}
        tone={
          soonest
            ? (soonest.daysLeft ?? 99) <= 3
              ? "danger"
              : (soonest.daysLeft ?? 99) <= 14
                ? "warn"
                : "ok"
            : undefined
        }
        foot={
          // B94：写明最先到期的是域名还是证书
          soonest
            ? `${t(soonest.kind === "domain" ? "Domain" : "Certificate")} ${soonest.name} ${t("expires soonest")}`
            : t("Nothing to check")
        }
      />
      <StatCard
        label={t("Scripts")}
        value={scripts.length}
        foot={t("Saved commands")}
      />
      <StatCard
        label={t("Subscriptions")}
        to="/monitoring/subscriptions"
        caption={
          spend ? (
            <span
              role="button"
              tabIndex={0}
              className="monitoring-currency"
              title={
                spend.converted
                  ? t("Switch currency")
                  : `${t("Exchange rates are not ready. Not counted:")} ${spend.missing.join(", ")}`
              }
              onClick={switchCurrency}
              onKeyDown={(e) =>
                (e.key === "Enter" || e.key === " ") && switchCurrency(e)
              }
            >
              {spend.currency}
            </span>
          ) : undefined
        }
        value={
          spend
            ? formatMoney(Math.round(spend.monthly), spend.currency)
            : subs.length
        }
        unit={spend ? t("per month") : undefined}
        foot={
          nextSub
            ? `${nextSub.name} · ${nextSub.daysLeft} ${t("days left")}`
            : t("No renewals coming up")
        }
      />
    </StatStrip>
  );
}

const CURRENCY_KEY = "xc.monitoring.currency";

function readCurrency(): SpendCurrency {
  try {
    const v = localStorage.getItem(CURRENCY_KEY);
    if (v === "CNY" || v === "USD") return v;
  } catch {
    // 读不了用默认的人民币
  }
  return "CNY";
}
