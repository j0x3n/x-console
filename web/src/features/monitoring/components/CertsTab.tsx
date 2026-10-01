import { Plus, ShieldCheck } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { useMonitors, type Monitor } from "../api";
import { expiryTone, groupByDomain, type DomainGroup } from "../lib";
import { longDate, useIdParam, useParam } from "./common";
import MonitorDetail from "./MonitorDetail";
import MonitorDialog from "./MonitorDialog";

/** 证书和域名到期监控。同一个主域名合成一行，快到期的排在前面（B50）。 */
export default function CertsTab() {
  const t = useT();
  const monitors = useMonitors();
  const [creating, setCreating] = useParam("new");
  const [openId, setOpenId] = useIdParam("monitor");
  const items = (monitors.data ?? []).filter((m) => m.kind !== "http");
  const groups = groupByDomain(items);
  const open = items.find((m) => m.id === openId) ?? null;

  return (
    <>
      {monitors.isPending ? (
        <Loading />
      ) : monitors.isError ? (
        <ErrorState error={monitors.error} onRetry={() => monitors.refetch()} />
      ) : groups.length === 0 ? (
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
          {groups.map((g) => (
            <DomainRow key={g.domain} group={g} onOpen={setOpenId} />
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

function DomainRow({
  group: g,
  onOpen,
}: {
  group: DomainGroup;
  onOpen: (id: number) => void;
}) {
  const t = useT();
  const language = useLanguage();
  // 点整行打开最快到期的那一项
  const first =
    [...g.items].sort(
      (a, b) => (a.daysLeft ?? Infinity) - (b.daysLeft ?? Infinity),
    )[0] ?? g.items[0];
  const errors = g.items.filter((m) => m.enabled && m.lastError);
  const tone = expiryTone(first.kind, g.daysLeft);
  return (
    <div
      className="monitoring-row domain-row"
      role="button"
      tabIndex={0}
      onClick={() => onOpen(first.id)}
      onKeyDown={(e) => e.key === "Enter" && onOpen(first.id)}
    >
      <span className={`xc-dot ${tone}`} />
      <span className="monitoring-row-main">
        <strong>{g.domain}</strong>
        <small className="xc-muted">
          {g.items
            .map((m) => (m.kind === "tls" ? m.target : t("Domain")))
            .join(" · ")}
        </small>
        {errors.map((m) => (
          <small key={m.id} className="monitoring-row-error">
            {m.lastError}
          </small>
        ))}
      </span>
      <span className="monitoring-expiry-chips">
        {g.items.map((m) => (
          <ExpiryChip key={m.id} monitor={m} onOpen={onOpen} />
        ))}
        {first.expiresAt && (
          <small className="xc-muted">
            {longDate(first.expiresAt, language)}
          </small>
        )}
      </span>
    </div>
  );
}

function ExpiryChip({
  monitor: m,
  onOpen,
}: {
  monitor: Monitor;
  onOpen: (id: number) => void;
}) {
  const t = useT();
  const days = m.daysLeft === undefined ? null : Math.floor(m.daysLeft);
  const tone = m.enabled ? expiryTone(m.kind, m.daysLeft) : "";
  const label = t(m.kind === "tls" ? "Certificate" : "Domain");
  return (
    <button
      type="button"
      className={`monitoring-expiry-chip ${tone}${m.enabled ? "" : " off"}`}
      title={m.target}
      onClick={(e) => {
        e.stopPropagation();
        onOpen(m.id);
      }}
    >
      {label}{" "}
      {!m.enabled
        ? t("Paused")
        : days === null
          ? m.lastError
            ? t("Lookup failed")
            : "–"
          : days <= 0
            ? t("Expired")
            : `${days} ${t("days")}`}
    </button>
  );
}
