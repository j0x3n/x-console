import { Plus, ShieldCheck } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { useMonitors, type Monitor } from "../api";
import { expiryTone, monitorTone } from "../lib";
import { longDate, useIdParam, useParam } from "./common";
import MonitorDetail from "./MonitorDetail";
import MonitorDialog from "./MonitorDialog";
import { StatusBadge } from "./StatusBadge";

/** 证书和域名到期监控。快到期的排在前面。 */
export default function CertsTab() {
  const t = useT();
  const monitors = useMonitors();
  const [creating, setCreating] = useParam("new");
  const [openId, setOpenId] = useIdParam("monitor");
  const items = (monitors.data ?? [])
    .filter((m) => m.kind !== "http")
    .sort(
      (a, b) =>
        (a.daysLeft ?? Infinity) - (b.daysLeft ?? Infinity) ||
        a.name.localeCompare(b.name),
    );
  const open = items.find((m) => m.id === openId) ?? null;

  return (
    <>
      {monitors.isPending ? (
        <Loading />
      ) : monitors.isError ? (
        <ErrorState error={monitors.error} onRetry={() => monitors.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          title={t("No certificates or domains yet")}
          icon={<ShieldCheck size={28} />}
        >
          <span>{t("You get a reminder before they expire.")}</span>
          <button
            className="xc-btn primary small"
            onClick={() => setCreating("1")}
          >
            <Plus size={14} /> {t("New monitor")}
          </button>
        </EmptyState>
      ) : (
        <div className="xc-card monitoring-list">
          {items.map((m) => (
            <CertRow key={m.id} monitor={m} onOpen={() => setOpenId(m.id)} />
          ))}
        </div>
      )}
      <MonitorDialog
        open={creating === "1"}
        onClose={() => setCreating(null)}
        kinds={["tls", "domain"]}
      />
      <MonitorDetail monitor={open} onClose={() => setOpenId(null)} />
    </>
  );
}

function CertRow({
  monitor: m,
  onOpen,
}: {
  monitor: Monitor;
  onOpen: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const tone = expiryTone(m.kind, m.daysLeft);
  const days = m.daysLeft === undefined ? null : Math.floor(m.daysLeft);
  return (
    <button className="monitoring-row" onClick={onOpen}>
      <span className={`xc-dot ${m.enabled ? tone : ""}`} />
      <span className="monitoring-row-main">
        <strong>
          {m.name}{" "}
          <span className="xc-badge">
            {t(m.kind === "tls" ? "Certificate" : "Domain")}
          </span>
        </strong>
        <small className="xc-mono">{m.target}</small>
        {m.lastError && m.enabled && (
          <small className="monitoring-row-error">{m.lastError}</small>
        )}
      </span>
      <span className="monitoring-row-side">
        {days === null ? (
          <StatusBadge monitor={m} tone={monitorTone(m)} />
        ) : (
          <span className={`xc-badge ${tone}`}>
            {days <= 0 ? t("Expired") : `${days} ${t("days left")}`}
          </span>
        )}
        {m.expiresAt && (
          <small className="xc-muted">{longDate(m.expiresAt, language)}</small>
        )}
      </span>
    </button>
  );
}
