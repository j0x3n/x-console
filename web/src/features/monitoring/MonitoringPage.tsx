import { NavLink, useParams } from "react-router";
import { Plus } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import { useMonitors } from "./api";
import CertsTab from "./components/CertsTab";
import { useParam } from "./components/common";
import ScriptsTab from "./components/ScriptsTab";
import SitesTab from "./components/SitesTab";
import SubscriptionsTab from "./components/SubscriptionsTab";

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
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Monitoring")}
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
      <nav className="xc-tabs monitoring-tabs">
        {tabs.map((item) => {
          const count =
            item.id === ""
              ? down.filter((m) => m.kind === "http").length
              : item.id === "certs"
                ? down.filter((m) => m.kind !== "http").length
                : 0;
          return (
            <NavLink
              key={item.id}
              to={item.to}
              end
              className={item.id === current.id ? "active" : ""}
            >
              {t(item.label)}
              {count > 0 && (
                <span className="xc-badge danger monitoring-tab-count">
                  {count}
                </span>
              )}
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
