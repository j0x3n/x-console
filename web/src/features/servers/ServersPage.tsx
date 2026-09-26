import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { BellRing, Plus, Server } from "lucide-react";
import { useServerEvent } from "../../api/events";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { applyMetricsEvent, useHosts } from "./api";
import { AlertHistoryCard, AlertRulesCard } from "./components/AlertRules";
import HostCard from "./components/HostCard";
import SshHostsDialog from "./components/SshHosts";

export default function ServersPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const hosts = useHosts("server");
  useServerEvent("host.metrics", applyMetricsEvent);
  const [sshOpen, setSshOpen] = useState(params.get("ssh") === "1");
  const alertsOpen = params.get("alerts") === "1";
  const closeAlerts = () => {
    params.delete("alerts");
    setParams(params, { replace: true });
  };
  const online = hosts.data?.filter((h) => h.online).length ?? 0;

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Servers")}
        subtitle={hosts.data ? `${hosts.data.length} ${t("machines")} · ${online} ${t("online")}` : undefined}
        aside={
          <>
            <button className="xc-btn" onClick={() => setParams({ alerts: "1" })}>
              <BellRing size={14} /> {t("Alerts")}
            </button>
            <button className="xc-btn" onClick={() => setSshOpen(true)}>
              <Plus size={14} /> {t("SSH hosts")}
            </button>
          </>
        }
      />
      {hosts.isPending ? (
        <Loading />
      ) : hosts.isError ? (
        <ErrorState error={hosts.error} onRetry={() => hosts.refetch()} />
      ) : hosts.data.length === 0 ? (
        <EmptyState title={t("No servers yet")} icon={<Server size={28} />}>
          <span>{t("Pair a server agent, or add a server over SSH.")}</span>
          <div className="xc-row">
            <Link className="xc-btn primary" to="/settings/devices">
              {t("Pair a device")}
            </Link>
            <button className="xc-btn" onClick={() => setSshOpen(true)}>
              {t("Add SSH host")}
            </button>
          </div>
        </EmptyState>
      ) : (
        <div className="servers-grid">
          {hosts.data.map((h) => (
            <HostCard key={h.id} host={h} />
          ))}
        </div>
      )}
      <SshHostsDialog open={sshOpen} onClose={() => setSshOpen(false)} />
      <Dialog open={alertsOpen} onClose={closeAlerts} title={t("Alerts")} wide>
        <div className="xc-stack">
          <AlertHistoryCard />
          <AlertRulesCard />
        </div>
        <div className="xc-dialog-actions">
          <button className="xc-btn primary" onClick={closeAlerts}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    </div>
  );
}
