import { NavLink, useParams } from "react-router";
import { Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import { useT } from "../../contexts/LanguageContext";
import { useMonitors, useScripts, useSubscriptions, useSubscriptionSummary } from "./api";
import CertsTab from "./components/CertsTab";
import { useParam } from "./components/common";
import ScriptsTab from "./components/ScriptsTab";
import SitesTab from "./components/SitesTab";
import SubscriptionsTab from "./components/SubscriptionsTab";

const tabs = [
  { id: "", label: "Websites", to: "/monitoring", add: "New website" },
  { id: "certs", label: "Certificates & domains", to: "/monitoring/certs", add: "New monitor" },
  { id: "scripts", label: "Scripts", to: "/monitoring/scripts", add: "New script" },
  { id: "subscriptions", label: "Subscriptions", to: "/monitoring/subscriptions", add: "New subscription" },
];

export default function MonitoringPage() {
  const t = useT();
  const { tab = "" } = useParams();
  const [, setCreating] = useParam("new");
  const current = tabs.find((x) => x.id === tab) ?? tabs[0];
  const monitors = useMonitors();
  const down = (monitors.data ?? []).filter((m) => m.enabled && m.lastStatus === "down");
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Monitoring")}
        subtitle={down.length > 0 ? `${down.length} ${t("monitors are down")}` : undefined}
        aside={
          <button className="xc-btn primary" onClick={() => setCreating("1")}>
            <Plus size={15} /> {t(current.add)}
          </button>
        }
      />
      <MonitoringStats />
      <nav className="xc-tabs monitoring-tabs">
        {tabs.map((item) => {
          const count = item.id === "" ? down.filter((m) => m.kind === "http").length : item.id === "certs" ? down.filter((m) => m.kind !== "http").length : 0;
          return (
            <NavLink key={item.id} to={item.to} end className={item.id === current.id ? "active" : ""}>
              {t(item.label)}
              {count > 0 && <span className="xc-badge danger monitoring-tab-count">{count}</span>}
            </NavLink>
          );
        })}
      </nav>
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
  const total = summary?.totals[0];
  return (
    <StatStrip label={t("Monitoring")}>
      <StatCard
        label={t("Websites")}
        caption={sites.length ? `${up}/${sites.length}` : undefined}
        value={sites.length ? up : "–"}
        unit={sites.length ? t("sites up") : undefined}
        tone={down ? "danger" : sites.length ? "ok" : undefined}
        foot={down ? `${down} ${t("monitors are down")}` : sites.length ? t("All websites up") : t("No websites yet")}
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
        tone={soonest ? ((soonest.daysLeft ?? 99) <= 3 ? "danger" : (soonest.daysLeft ?? 99) <= 14 ? "warn" : "ok") : undefined}
        foot={soonest ? `${soonest.name} ${t("expires soonest")}` : t("Nothing to check")}
      />
      <StatCard label={t("Scripts")} value={scripts.length} foot={t("Saved commands")} />
      <StatCard
        label={t("Subscriptions")}
        caption={total ? total.currency : undefined}
        value={total ? Math.round(total.monthly) : subs.length}
        unit={total ? t("per month") : undefined}
        foot={nextSub ? `${nextSub.name} · ${nextSub.daysLeft} ${t("days left")}` : t("No renewals coming up")}
      />
    </StatStrip>
  );
}
