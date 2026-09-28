import { useMemo, useState } from "react";
import { FileText, Play, RefreshCw, RotateCw, Square } from "lucide-react";
import { errorMessage, unwrap } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import Dialog from "../../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  hostsApi,
  hostsKeys,
  useHostMutation,
  useServiceLogs,
  useServices,
  type HostDetail,
  type Service,
  type ServiceAction,
} from "../api";
import { confirmAction } from "../../../components/ui/ConfirmDialog";

type StateFilter = "all" | "running" | "stopped" | "failed";

export function filterServices(
  items: Service[],
  query: string,
  state: StateFilter,
): Service[] {
  const q = query.trim().toLowerCase();
  return items.filter(
    (s) =>
      (state === "all" || s.state === state) &&
      (!q ||
        s.name.toLowerCase().includes(q) ||
        s.description.toLowerCase().includes(q)),
  );
}

const stateTone: Record<string, string> = {
  running: "ok",
  failed: "danger",
  starting: "info",
  stopping: "warn",
};

const serviceTitle: Partial<Record<ServiceAction, string>> = {
  stop: "Stop service",
  restart: "Restart service",
  disable: "Disable service",
};

export default function ServicesTab({ host }: { host: HostDetail }) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [state, setState] = useState<StateFilter>("all");
  const [logsFor, setLogsFor] = useState<string | null>(null);
  const services = useServices(host.id);
  const action = useHostMutation(
    ({ name, act }: { name: string; act: ServiceAction }) =>
      withElevation(() =>
        unwrap(
          hostsApi.POST("/hosts/{hostId}/services/{name}/{action}", {
            params: { path: { hostId: host.id, name, action: act } },
          }),
        ),
      ),
    [hostsKeys.services(host.id)],
  );
  const items = useMemo(
    () => filterServices(services.data?.items ?? [], query, state),
    [services.data, query, state],
  );
  const run = async (s: Service, act: ServiceAction) => {
    if (
      (act === "stop" || act === "disable" || act === "restart") &&
      !(await confirmAction({
        title: `${t(serviceTitle[act] ?? "")} ${s.name}？`,
        description:
          act === "disable"
            ? t("It no longer starts with the system.")
            : act === "stop"
              ? t("The service stops until you start it again.")
              : t("The service is unavailable for a moment."),
        confirmLabel: t(
          act === "stop" ? "Stop" : act === "disable" ? "Disable" : "Restart",
        ),
      }))
    )
      return;
    action.mutate(
      { name: s.name, act },
      {
        onSuccess: () => toast(t("Done")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Services")}</h2>
        <div className="xc-row servers-wrap">
          <input
            className="xc-input servers-search"
            placeholder={t("Filter services")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select
            className="xc-select servers-narrow"
            value={state}
            onChange={(e) => setState(e.target.value as StateFilter)}
            aria-label={t("State")}
          >
            <option value="all">{t("All")}</option>
            <option value="running">{t("Running")}</option>
            <option value="stopped">{t("Stopped")}</option>
            <option value="failed">{t("Failed")}</option>
          </select>
          <button
            className="xc-btn small"
            onClick={() => services.refetch()}
            disabled={services.isFetching}
            aria-label={t("Refresh")}
          >
            <RefreshCw
              size={14}
              className={services.isFetching ? "servers-spin" : ""}
            />
          </button>
        </div>
      </div>
      {services.isPending ? (
        <Loading />
      ) : services.isError ? (
        <ErrorState error={services.error} onRetry={() => services.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState title={t("No matching services")} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table servers-table">
            <thead>
              <tr>
                <th>{t("Name")}</th>
                <th>{t("State")}</th>
                <th>{t("Start at boot")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((s) => (
                <tr key={s.name}>
                  <td className="servers-cmd">
                    <strong>{s.name}</strong>
                    {s.description && (
                      <small className="xc-muted">{s.description}</small>
                    )}
                  </td>
                  <td>
                    <span className={`xc-badge ${stateTone[s.state] ?? ""}`}>
                      {t(stateLabel(s.state))}
                    </span>
                  </td>
                  <td>
                    <button
                      className="xc-btn small ghost"
                      disabled={action.isPending}
                      onClick={() => run(s, s.enabled ? "disable" : "enable")}
                      title={t(s.enabled ? "Disable" : "Enable")}
                    >
                      {s.enabled ? t("Yes") : t("No")}
                      {s.startType && (
                        <small className="xc-muted"> · {s.startType}</small>
                      )}
                    </button>
                  </td>
                  <td className="servers-actions">
                    {s.state === "running" ? (
                      <>
                        <button
                          className="xc-btn small"
                          disabled={action.isPending}
                          onClick={() => run(s, "restart")}
                        >
                          <RotateCw size={13} /> {t("Restart")}
                        </button>
                        <button
                          className="xc-btn small danger"
                          disabled={action.isPending}
                          onClick={() => run(s, "stop")}
                        >
                          <Square size={12} /> {t("Stop")}
                        </button>
                      </>
                    ) : (
                      <button
                        className="xc-btn small"
                        disabled={action.isPending}
                        onClick={() => run(s, "start")}
                      >
                        <Play size={13} /> {t("Start")}
                      </button>
                    )}
                    <button
                      className="xc-btn small ghost"
                      onClick={() => setLogsFor(s.name)}
                      aria-label={t("Logs")}
                    >
                      <FileText size={13} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <LogsDialog
        hostId={host.id}
        name={logsFor}
        onClose={() => setLogsFor(null)}
      />
    </div>
  );
}

function stateLabel(state: string) {
  switch (state) {
    case "running":
      return "Running";
    case "stopped":
      return "Stopped";
    case "failed":
      return "Failed";
    case "starting":
      return "Starting";
    case "stopping":
      return "Stopping";
  }
  return "Other";
}

function LogsDialog({
  hostId,
  name,
  onClose,
}: {
  hostId: string;
  name: string | null;
  onClose: () => void;
}) {
  const t = useT();
  const logs = useServiceLogs(hostId, name);
  return (
    <Dialog
      open={!!name}
      onClose={onClose}
      title={`${t("Logs")} · ${name ?? ""}`}
      wide
    >
      {logs.isPending ? (
        <Loading />
      ) : logs.isError ? (
        <ErrorState error={logs.error} onRetry={() => logs.refetch()} />
      ) : logs.data.lines.length === 0 ? (
        <EmptyState title={t("No log lines")} />
      ) : (
        <pre className="servers-logs">{logs.data.lines.join("\n")}</pre>
      )}
      <div className="xc-dialog-actions">
        <button
          className="xc-btn"
          onClick={() => logs.refetch()}
          disabled={logs.isFetching}
        >
          <RefreshCw size={14} /> {t("Refresh")}
        </button>
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}
