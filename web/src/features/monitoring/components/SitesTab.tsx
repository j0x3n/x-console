import { Globe, Plus } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import { useMonitors, type Monitor } from "../api";
import { monitorTone } from "../lib";
import { useIdParam, useParam } from "./common";
import MonitorDetail from "./MonitorDetail";
import MonitorDialog from "./MonitorDialog";
import { StatusBadge } from "./StatusBadge";

/** 网站监控列表。 */
export default function SitesTab() {
  const t = useT();
  const monitors = useMonitors();
  const [creating, setCreating] = useParam("new");
  const [openId, setOpenId] = useIdParam("monitor");
  const items = (monitors.data ?? []).filter((m) => m.kind === "http");
  const open = items.find((m) => m.id === openId) ?? null;

  return (
    <>
      {monitors.isPending ? (
        <Loading />
      ) : monitors.isError ? (
        <ErrorState error={monitors.error} onRetry={() => monitors.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState title={t("No websites yet")} icon={<Globe size={28} />}>
          <span>{t("Add a site and it is checked every minute.")}</span>
          <button
            className="xc-btn primary small"
            onClick={() => setCreating("1")}
          >
            <Plus size={14} /> {t("New website")}
          </button>
        </EmptyState>
      ) : (
        <div className="xc-card monitoring-list">
          {items.map((m) => (
            <SiteRow key={m.id} monitor={m} onOpen={() => setOpenId(m.id)} />
          ))}
        </div>
      )}
      <MonitorDialog
        open={creating === "1"}
        onClose={() => setCreating(null)}
        kinds={["http"]}
      />
      <MonitorDetail monitor={open} onClose={() => setOpenId(null)} />
    </>
  );
}

function SiteRow({
  monitor: m,
  onOpen,
}: {
  monitor: Monitor;
  onOpen: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const tone = monitorTone(m);
  return (
    <button className="monitoring-row" onClick={onOpen}>
      <span className={`xc-dot ${tone}`} />
      <span className="monitoring-row-main">
        <strong>{m.name}</strong>
        <small className="xc-mono">{m.target}</small>
        {m.lastError && m.enabled && (
          <small className="monitoring-row-error">{m.lastError}</small>
        )}
      </span>
      <span className="monitoring-row-side">
        <StatusBadge monitor={m} tone={tone} />
        {m.lastCheckedAt && (
          <small className="xc-muted">
            {relativeTime(m.lastCheckedAt, language)}
          </small>
        )}
      </span>
    </button>
  );
}
