import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { errorMessage, unwrap } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  hostsApi,
  hostsKeys,
  useAlertRules,
  useAlerts,
  useHosts,
  type AlertRule,
  type AlertRuleInput,
} from "../api";

const metricLabels: Record<string, string> = {
  cpu: "CPU",
  memory: "Memory",
  disk: "Disk",
  offline: "Offline",
};

/** 规则的一句话描述，例如 “CPU > 90%，持续 5 分钟”。 */
export function describeRule(
  rule: Pick<AlertRule, "metric" | "op" | "threshold" | "durationSeconds">,
  t: (s: string) => string,
): string {
  const mins = Math.round(rule.durationSeconds / 60);
  const lasting =
    rule.durationSeconds > 0 ? `，${t("for")} ${mins || "<1"} ${t("min")}` : "";
  if (rule.metric === "offline")
    return rule.durationSeconds > 0
      ? `${t("Offline")} ${t("over")} ${mins || "<1"} ${t("min")}`
      : t("Offline");
  return `${t(metricLabels[rule.metric])} ${rule.op === "lt" ? "<" : ">"} ${rule.threshold}%${lasting}`;
}

/** 告警规则列表和编辑。hostId 给了就只看这台机器的规则和通用规则。 */
export function AlertRulesCard({ hostId }: { hostId?: string }) {
  const t = useT();
  const qc = useQueryClient();
  const rules = useAlertRules();
  const hosts = useHosts();
  const [editing, setEditing] = useState<AlertRule | "new" | null>(null);
  const names = new Map((hosts.data ?? []).map((h) => [h.id, h.name]));
  const shown = (rules.data ?? []).filter(
    (r) => !hostId || !r.hostId || r.hostId === hostId,
  );
  const remove = async (r: AlertRule) => {
    if (!confirm(t("Delete this rule?"))) return;
    try {
      await unwrap(
        hostsApi.DELETE("/alert-rules/{ruleId}", {
          params: { path: { ruleId: r.id } },
        }),
      );
      qc.invalidateQueries({ queryKey: hostsKeys.rules });
      toast(t("Deleted"));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Alert rules")}</h2>
        <button className="xc-btn small" onClick={() => setEditing("new")}>
          <Plus size={14} /> {t("New rule")}
        </button>
      </div>
      {rules.isPending ? (
        <Loading />
      ) : rules.isError ? (
        <ErrorState error={rules.error} onRetry={() => rules.refetch()} />
      ) : shown.length === 0 ? (
        <EmptyState title={t("No alert rules yet")}>
          <span>
            {t(
              "Add a rule to get notified when CPU, memory or disk run high, or a machine goes offline.",
            )}
          </span>
        </EmptyState>
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table servers-table">
            <thead>
              <tr>
                <th>{t("Rule")}</th>
                <th>{t("Machine")}</th>
                <th>{t("Severity")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => (
                <tr key={r.id} className={r.enabled ? "" : "servers-disabled"}>
                  <td>
                    {describeRule(r, t)}
                    {!r.enabled && (
                      <span className="xc-badge"> {t("Paused")}</span>
                    )}
                  </td>
                  <td>
                    {r.hostId
                      ? (names.get(r.hostId) ?? r.hostId)
                      : t("All machines")}
                  </td>
                  <td>
                    <span
                      className={`xc-badge ${r.severity === "critical" ? "danger" : "warn"}`}
                    >
                      {t(r.severity === "critical" ? "Critical" : "Warning")}
                    </span>
                  </td>
                  <td className="servers-actions">
                    <button
                      className="xc-btn small ghost"
                      onClick={() => setEditing(r)}
                      aria-label={t("Edit")}
                    >
                      <Pencil size={13} />
                    </button>
                    <button
                      className="xc-btn small ghost"
                      onClick={() => remove(r)}
                      aria-label={t("Delete")}
                    >
                      <Trash2 size={13} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <RuleDialog
          rule={editing === "new" ? null : editing}
          hostId={hostId}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

function RuleDialog({
  rule,
  hostId,
  onClose,
}: {
  rule: AlertRule | null;
  hostId?: string;
  onClose: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const hosts = useHosts();
  const [form, setForm] = useState<AlertRuleInput>(
    rule
      ? {
          hostId: rule.hostId,
          metric: rule.metric,
          op: rule.op,
          threshold: rule.threshold,
          durationSeconds: rule.durationSeconds,
          severity: rule.severity,
          enabled: rule.enabled,
        }
      : {
          hostId,
          metric: "cpu",
          op: "gt",
          threshold: 90,
          durationSeconds: 300,
          severity: "warning",
          enabled: true,
        },
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (patch: Partial<AlertRuleInput>) =>
    setForm((f) => ({ ...f, ...patch }));
  const offline = form.metric === "offline";
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    const body = { ...form, hostId: form.hostId || undefined };
    try {
      if (rule) {
        await unwrap(
          hostsApi.PUT("/alert-rules/{ruleId}", {
            params: { path: { ruleId: rule.id } },
            body,
          }),
        );
      } else {
        await unwrap(hostsApi.POST("/alert-rules", { body }));
      }
      qc.invalidateQueries({ queryKey: hostsKeys.rules });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={t(rule ? "Edit rule" : "New rule")}
      description={t(
        "A notification is sent when the condition holds for the whole duration.",
      )}
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Machine")}</span>
          <select
            className="xc-select"
            value={form.hostId ?? ""}
            onChange={(e) => set({ hostId: e.target.value || undefined })}
          >
            <option value="">{t("All machines")}</option>
            {(hosts.data ?? []).map((h) => (
              <option key={h.id} value={h.id}>
                {h.name}
              </option>
            ))}
          </select>
        </label>
        <div className="servers-form-row">
          <label className="xc-field">
            <span>{t("Metric")}</span>
            <select
              className="xc-select"
              value={form.metric}
              onChange={(e) =>
                set({ metric: e.target.value as AlertRuleInput["metric"] })
              }
            >
              {Object.entries(metricLabels).map(([k, v]) => (
                <option key={k} value={k}>
                  {t(v)}
                </option>
              ))}
            </select>
          </label>
          {!offline && (
            <>
              <label className="xc-field">
                <span>{t("Condition")}</span>
                <select
                  className="xc-select"
                  value={form.op}
                  onChange={(e) =>
                    set({ op: e.target.value as AlertRuleInput["op"] })
                  }
                >
                  <option value="gt">{t("Above")}</option>
                  <option value="lt">{t("Below")}</option>
                </select>
              </label>
              <label className="xc-field">
                <span>{t("Threshold (%)")}</span>
                <input
                  className="xc-input"
                  type="number"
                  min={0}
                  max={100}
                  step={1}
                  value={form.threshold ?? 0}
                  onChange={(e) => set({ threshold: Number(e.target.value) })}
                  required
                />
              </label>
            </>
          )}
        </div>
        <div className="servers-form-row">
          <label className="xc-field">
            <span>
              {t(offline ? "Offline for (minutes)" : "Lasting (minutes)")}
            </span>
            <input
              className="xc-input"
              type="number"
              min={0}
              step={1}
              value={Math.round((form.durationSeconds ?? 0) / 60)}
              onChange={(e) =>
                set({ durationSeconds: Number(e.target.value) * 60 })
              }
            />
          </label>
          <label className="xc-field">
            <span>{t("Severity")}</span>
            <select
              className="xc-select"
              value={form.severity}
              onChange={(e) =>
                set({ severity: e.target.value as AlertRuleInput["severity"] })
              }
            >
              <option value="warning">{t("Warning")}</option>
              <option value="critical">{t("Critical")}</option>
            </select>
          </label>
        </div>
        <label className="servers-check">
          <input
            type="checkbox"
            checked={form.enabled ?? true}
            onChange={(e) => set({ enabled: e.target.checked })}
          />{" "}
          {t("Enabled")}
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={busy}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** 告警历史。 */
export function AlertHistoryCard({ hostId }: { hostId?: string }) {
  const t = useT();
  const language = useLanguage();
  const alerts = useAlerts(hostId);
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Alert history")}</h2>
        <span className="xc-muted">{t("Last 7 days")}</span>
      </div>
      {alerts.isPending ? (
        <Loading />
      ) : alerts.isError ? (
        <ErrorState error={alerts.error} onRetry={() => alerts.refetch()} />
      ) : alerts.data.items.length === 0 ? (
        <EmptyState title={t("No alerts")} />
      ) : (
        <ul className="servers-alerts">
          {alerts.data.items.map((a) => (
            <li key={a.id}>
              <span
                className={`xc-badge ${a.resolvedAt ? "ok" : a.severity === "critical" ? "danger" : "warn"}`}
              >
                {t(a.resolvedAt ? "Resolved" : "Firing")}
              </span>
              <div>
                <strong>
                  {hostId ? "" : `${a.hostName} · `}
                  {a.message}
                </strong>
                <small className="xc-muted">
                  {relativeTime(a.firedAt, language)}
                  {a.resolvedAt &&
                    ` · ${t("resolved")} ${relativeTime(a.resolvedAt, language)}`}
                </small>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export default function AlertsTab({ host }: { host: { id: string } }) {
  return (
    <div className="xc-stack">
      <AlertHistoryCard hostId={host.id} />
      <AlertRulesCard hostId={host.id} />
    </div>
  );
}
