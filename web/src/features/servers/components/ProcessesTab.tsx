import { useMemo, useState } from "react";
import { ChevronRight, RefreshCw, X } from "lucide-react";
import MoreMenu from "../../../components/ui/MoreMenu";
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
import { confirmAction } from "../../../components/ui/ConfirmDialog";

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

export interface ProcessGroup {
  name: string;
  procs: Process[];
  cpu: number;
  memRss: number;
  minPid: number;
  startedAt?: string;
  users: string[];
}

/** 按进程名合并（B28）。CPU 和内存求和，启动时间取最早的。 */
export function groupProcesses(
  items: Process[],
  sort: ProcessSort,
): ProcessGroup[] {
  const map = new Map<string, ProcessGroup>();
  for (const p of items) {
    let g = map.get(p.name);
    if (!g) {
      g = {
        name: p.name,
        procs: [],
        cpu: 0,
        memRss: 0,
        minPid: p.pid,
        users: [],
      };
      map.set(p.name, g);
    }
    g.procs.push(p);
    g.cpu += p.cpu;
    g.memRss += p.memRss;
    g.minPid = Math.min(g.minPid, p.pid);
    if (!g.users.includes(p.user)) g.users.push(p.user);
    const started =
      p.startedAt && !p.startedAt.startsWith("0001") ? p.startedAt : undefined;
    if (started && (!g.startedAt || started < g.startedAt))
      g.startedAt = started;
  }
  const groups = [...map.values()];
  const by: Record<ProcessSort, (a: ProcessGroup, b: ProcessGroup) => number> =
    {
      cpu: (a, b) => b.cpu - a.cpu,
      mem: (a, b) => b.memRss - a.memRss,
      pid: (a, b) => a.minPid - b.minPid,
      name: (a, b) => a.name.localeCompare(b.name),
    };
  return groups.sort(by[sort]);
}

const MERGE_KEY = "xc.hosts.mergeProcs";
function readMerge(): boolean {
  try {
    return localStorage.getItem(MERGE_KEY) !== "0";
  } catch {
    return true;
  }
}

export default function ProcessesTab({ host }: { host: HostDetail }) {
  const t = useT();
  const language = useLanguage();
  const [sort, setSort] = useState<ProcessSort>("cpu");
  const [query, setQuery] = useState("");
  const [merge, setMergeState] = useState(readMerge);
  const [open, setOpen] = useState<Set<string>>(new Set());
  const setMerge = (v: boolean) => {
    setMergeState(v);
    try {
      localStorage.setItem(MERGE_KEY, v ? "1" : "0");
    } catch {
      /* 记不住就算了 */
    }
  };
  // 合并时多取一些，同名进程不被截断。
  const procs = useProcesses(host.id, sort, merge ? 1000 : 300);
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
  const items = useMemo(
    () => filterProcesses(procs.data?.items ?? [], query),
    [procs.data, query],
  );
  const groups = useMemo(
    () => (merge ? groupProcesses(items, sort) : []),
    [merge, items, sort],
  );
  const toggleOpen = (name: string) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  const killGroup = async (g: ProcessGroup) => {
    if (
      !(await confirmAction({
        title: `结束全部 ${g.procs.length} 个 ${g.name} 进程？`,
        description: t("Unsaved work in it may be lost."),
        confirmLabel: t("End process"),
      }))
    )
      return;
    let failed = 0;
    for (const p of g.procs) {
      try {
        await kill.mutateAsync({ pid: p.pid, signal: "TERM" });
      } catch {
        failed++;
      }
    }
    toast(
      failed
        ? {
            message: `${failed} ${t("processes could not be ended")}`,
            tone: "error",
          }
        : t("Signal sent"),
    );
  };
  const onKill = async (p: Process, signal: string) => {
    const msg =
      signal === "KILL"
        ? `强制结束 ${p.name}（${p.pid}）？`
        : `结束 ${p.name}（${p.pid}）？`;
    if (
      !(await confirmAction({
        title: msg,
        description: t("Unsaved work in it may be lost."),
        confirmLabel: signal === "KILL" ? t("Force end") : t("End process"),
      }))
    )
      return;
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
      <button
        className={`servers-sort${sort === key ? " active" : ""}`}
        onClick={() => setSort(key)}
      >
        {label}
      </button>
    </th>
  );

  const rowProps = {
    windows: host.os === "windows",
    busy: kill.isPending,
    onKill,
  };

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>
          {t("Processes")}{" "}
          {procs.data && <span className="xc-muted">{procs.data.total}</span>}
        </h2>
        <div className="xc-row">
          <label className="xc-check servers-merge">
            <input
              type="checkbox"
              checked={merge}
              onChange={(e) => setMerge(e.target.checked)}
            />
            <span>{t("Merge same names")}</span>
          </label>
          <input
            className="xc-input servers-search"
            placeholder={t("Filter by name, user or PID")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button
            className="xc-btn small"
            onClick={() => procs.refetch()}
            disabled={procs.isFetching}
            aria-label={t("Refresh")}
          >
            <RefreshCw
              size={14}
              className={procs.isFetching ? "servers-spin" : ""}
            />
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
              {merge
                ? groups.map((g) =>
                    g.procs.length === 1 ? (
                      <ProcessRow key={g.name} p={g.procs[0]} {...rowProps} />
                    ) : (
                      <GroupRows
                        key={g.name}
                        group={g}
                        open={open.has(g.name)}
                        onToggle={() => toggleOpen(g.name)}
                        onKillGroup={() => killGroup(g)}
                        {...rowProps}
                      />
                    ),
                  )
                : items.map((p) => (
                    <ProcessRow key={p.pid} p={p} {...rowProps} />
                  ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

interface RowProps {
  windows: boolean;
  busy: boolean;
  onKill: (p: Process, signal: string) => void;
}

function ProcessRow({
  p,
  windows,
  busy,
  onKill,
  child,
}: RowProps & { p: Process; child?: boolean }) {
  const t = useT();
  const language = useLanguage();
  return (
    <tr className={child ? "servers-proc-child" : undefined}>
      <td className="xc-mono">{p.pid}</td>
      <td className="servers-cmd" title={p.cmdline}>
        <strong>{p.name}</strong>
        {p.cmdline && <small className="xc-muted xc-mono">{p.cmdline}</small>}
      </td>
      <td>{p.user}</td>
      <td className="servers-num">{p.cpu.toFixed(1)}%</td>
      <td className="servers-num">{formatBytes(p.memRss)}</td>
      <td className="xc-muted">
        {p.startedAt && !p.startedAt.startsWith("0001")
          ? relativeTime(p.startedAt, language)
          : "—"}
      </td>
      <td className="servers-actions">
        <button
          className="xc-btn small danger"
          disabled={busy}
          onClick={() => onKill(p, "TERM")}
          title={t("End process")}
        >
          <X size={13} /> {t("End")}
        </button>
        {!windows && (
          <button
            className="xc-btn small ghost"
            disabled={busy}
            onClick={() => onKill(p, "KILL")}
          >
            {t("Force")}
          </button>
        )}
      </td>
    </tr>
  );
}

function GroupRows({
  group: g,
  open,
  onToggle,
  onKillGroup,
  ...rowProps
}: RowProps & {
  group: ProcessGroup;
  open: boolean;
  onToggle: () => void;
  onKillGroup: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  return (
    <>
      <tr className="servers-proc-group" onClick={onToggle}>
        <td className="xc-mono xc-muted">
          <ChevronRight
            size={13}
            className={`servers-proc-chevron${open ? " open" : ""}`}
          />
        </td>
        <td className="servers-cmd">
          <strong>
            {g.name} <span className="xc-badge">×{g.procs.length}</span>
          </strong>
        </td>
        <td>{g.users.join(", ")}</td>
        <td className="servers-num">{g.cpu.toFixed(1)}%</td>
        <td className="servers-num">{formatBytes(g.memRss)}</td>
        <td className="xc-muted">
          {g.startedAt ? relativeTime(g.startedAt, language) : "—"}
        </td>
        <td className="servers-actions" onClick={(e) => e.stopPropagation()}>
          <button
            type="button"
            className="xc-btn small ghost"
            aria-expanded={open}
            aria-label={`${open ? t("Collapse group") : t("Expand group")} ${g.name}`}
            onClick={onToggle}
          >
            {open ? t("Collapse group") : t("Expand group")}
          </button>
          <MoreMenu
            label={`${t("More")}：${g.name}`}
            title={g.name}
            items={[
              {
                key: "kill",
                label: t("End all"),
                icon: <X size={14} />,
                danger: true,
                onSelect: onKillGroup,
              },
            ]}
          />
        </td>
      </tr>
      {open &&
        g.procs.map((p) => (
          <ProcessRow key={p.pid} p={p} child {...rowProps} />
        ))}
    </>
  );
}
