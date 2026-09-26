import { useMemo, useState } from "react";
import { RefreshCw, X } from "lucide-react";
import { errorMessage, unwrap } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes, relativeTime } from "../../../lib/time";
import {
  hostsApi,
  hostsKeys,
  useHostMutation,
  useProcesses,
  type HostDetail,
  type Process,
  type ProcessSort,
} from "../api";

export function filterProcesses(items: Process[], query: string): Process[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items.filter(
    (p) =>
      p.name.toLowerCase().includes(q) ||
      p.cmdline.toLowerCase().includes(q) ||
      p.user.toLowerCase().includes(q) ||
      String(p.pid) === q,
  );
}

export default function ProcessesTab({ host }: { host: HostDetail }) {
  const t = useT();
  const language = useLanguage();
  const [sort, setSort] = useState<ProcessSort>("cpu");
  const [query, setQuery] = useState("");
  const procs = useProcesses(host.id, sort);
  const kill = useHostMutation(
    ({ pid, signal }: { pid: number; signal: string }) =>
      withElevation(() =>
        unwrap(
          hostsApi.POST("/hosts/{hostId}/processes/{pid}/kill", {
            params: { path: { hostId: host.id, pid } },
            body: { signal },
          }),
        ),
      ),
    [hostsKeys.processes(host.id)],
  );
  const items = useMemo(() => filterProcesses(procs.data?.items ?? [], query), [procs.data, query]);
  const onKill = (p: Process, signal: string) => {
    const msg = signal === "KILL" ? `强制结束 ${p.name}（${p.pid}）？` : `结束 ${p.name}（${p.pid}）？`;
    if (!confirm(msg)) return;
    kill.mutate(
      { pid: p.pid, signal },
      {
        onSuccess: () => toast(t("Signal sent")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };
  const sortHeader = (key: ProcessSort, label: string) => (
    <th>
      <button className={`servers-sort${sort === key ? " active" : ""}`} onClick={() => setSort(key)}>
        {label}
      </button>
    </th>
  );

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>
          {t("Processes")} {procs.data && <span className="xc-muted">{procs.data.total}</span>}
        </h2>
        <div className="xc-row">
          <input className="xc-input servers-search" placeholder={t("Filter by name, user or PID")} value={query} onChange={(e) => setQuery(e.target.value)} />
          <button className="xc-btn small" onClick={() => procs.refetch()} disabled={procs.isFetching} aria-label={t("Refresh")}>
            <RefreshCw size={14} className={procs.isFetching ? "servers-spin" : ""} />
          </button>
        </div>
      </div>
      {procs.isPending ? (
        <Loading />
      ) : procs.isError ? (
        <ErrorState error={procs.error} onRetry={() => procs.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState title={t("No matching processes")} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table servers-table">
            <thead>
              <tr>
                {sortHeader("pid", "PID")}
                {sortHeader("name", t("Name"))}
                <th>{t("User")}</th>
                {sortHeader("cpu", "CPU")}
                {sortHeader("mem", t("Memory"))}
                <th>{t("Started")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.pid}>
                  <td className="xc-mono">{p.pid}</td>
                  <td className="servers-cmd" title={p.cmdline}>
                    <strong>{p.name}</strong>
                    {p.cmdline && <small className="xc-muted xc-mono">{p.cmdline}</small>}
                  </td>
                  <td>{p.user}</td>
                  <td className="servers-num">{p.cpu.toFixed(1)}%</td>
                  <td className="servers-num">{formatBytes(p.memRss)}</td>
                  <td className="xc-muted">{p.startedAt && !p.startedAt.startsWith("0001") ? relativeTime(p.startedAt, language) : "—"}</td>
                  <td className="servers-actions">
                    <button className="xc-btn small danger" disabled={kill.isPending} onClick={() => onKill(p, "TERM")} title={t("End process")}>
                      <X size={13} /> {t("End")}
                    </button>
                    {host.os !== "windows" && (
                      <button className="xc-btn small ghost" disabled={kill.isPending} onClick={() => onKill(p, "KILL")}>
                        {t("Force")}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
