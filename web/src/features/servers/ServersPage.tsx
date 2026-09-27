import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { BellRing, Plus, Server } from "lucide-react";
import { useServerEvent } from "../../api/events";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import { MiniBars, Segments, StatCard, StatStrip } from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { applyMetricsEvent, useHosts, type Host } from "./api";
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
  const list = hosts.data ?? [];
  const alerts = list.reduce((sum, h) => sum + h.activeAlerts, 0);

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Servers")}
        subtitle={hosts.data ? `${hosts.data.length} ${t("machines")} · ${online} ${t("online")}` : undefined}
        meta={
          list.length > 0 && (
            <>
              <span className={`xc-dot ${alerts ? "danger" : online === list.length ? "ok" : "warn"}`} />
              {alerts
                ? `${alerts} ${t("active alerts")}`
                : online === list.length
                  ? t("All servers online")
                  : `${list.length - online} ${t("machines offline")}`}
            </>
          )
        }
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
        <>
        <ServerStats hosts={list} />
        <div className="servers-grid">
          {hosts.data.map((h) => (
            <HostCard key={h.id} host={h} />
          ))}
        </div>
        </>
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

const avg = (values: number[]) =>
  values.length ? values.reduce((a, b) => a + b, 0) / values.length : 0;

function ServerStats({ hosts }: { hosts: Host[] }) {
  const t = useT();
  const live = hosts.filter((h) => h.online);
  const online = live.length;
  const cpu = live.map((h) => h.cpu ?? 0);
  const mem = live.map((h) => h.memory ?? 0);
  const fullest = [...live].sort((a, b) => (b.disk ?? 0) - (a.disk ?? 0))[0];
  const alerts = hosts.reduce((sum, h) => sum + h.activeAlerts, 0);
  const level = (v: number) => (v >= 90 ? "danger" : v >= 75 ? "warn" : undefined);
  return (
    <StatStrip label={t("Servers")}>
      <StatCard
        label={t("Online")}
        caption={`${online}/${hosts.length}`}
        value={online}
        unit={t("machines")}
        foot={online === hosts.length ? t("All servers online") : `${hosts.length - online} ${t("machines offline")}`}
      >
        <Segments
          parts={[
            { value: online, tone: "ok", label: t("Online") },
            { value: hosts.length - online, tone: "danger", label: t("Offline") },
          ]}
        />
      </StatCard>
      <StatCard
        label={t("Average CPU")}
        value={online ? Math.round(avg(cpu)) : "–"}
        unit="%"
        tone={level(avg(cpu))}
      >
        <MiniBars values={cpu.map((v) => Math.round(v))} max={100} labels={live.map((h) => h.name.slice(0, 3))} />
      </StatCard>
      <StatCard
        label={t("Average memory")}
        value={online ? Math.round(avg(mem)) : "–"}
        unit="%"
        tone={level(avg(mem))}
      >
        <MiniBars values={mem.map((v) => Math.round(v))} max={100} tone="info" labels={live.map((h) => h.name.slice(0, 3))} />
      </StatCard>
      <StatCard
        label={t("Fullest disk")}
        value={fullest?.disk != null ? Math.round(fullest.disk) : "–"}
        unit="%"
        tone={level(fullest?.disk ?? 0)}
        foot={fullest?.name ?? t("No data")}
      />
      <StatCard
        label={t("Alerts")}
        value={alerts}
        tone={alerts ? "danger" : "ok"}
        foot={alerts ? t("Firing now") : t("No active alerts")}
      />
    </StatStrip>
  );
}
