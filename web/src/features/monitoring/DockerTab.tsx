import { useEffect, useMemo, useRef, useState } from "react";
import {
  FileText,
  Play,
  RefreshCw,
  RotateCw,
  Square,
  Trash2,
} from "lucide-react";
import { wsUrl } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import Dialog from "../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatBytes } from "../../lib/time";
import "./i18n";
import "./monitoring.css";
import {
  useContainerAction,
  useContainers,
  useDockerImages,
  useDockerStats,
  type ContainerAction,
} from "./api";
import { showError } from "./components/common";
import {
  appendLog,
  containerTone,
  emptyLog,
  formatPorts,
  logText,
  mergeStats,
  type ContainerRow,
} from "./lib";
import { confirmAction } from "../../components/ui/ConfirmDialog";

/** 服务器详情页的“容器”标签。机器上报了 docker 能力才显示。 */
const containerTitle: Record<ContainerAction, string> = {
  start: "Start container",
  stop: "Stop container",
  restart: "Restart container",
  remove: "Delete container",
};
const containerVerb: Record<ContainerAction, string> = {
  start: "Start",
  stop: "Stop",
  restart: "Restart",
  remove: "Delete",
};

export default function DockerTab({ host }: { host: { id: string } }) {
  const t = useT();
  const [all, setAll] = useState(false);
  const [showImages, setShowImages] = useState(false);
  const [logsFor, setLogsFor] = useState<ContainerRow | null>(null);
  const containers = useContainers(host.id, all);
  const stats = useDockerStats(host.id);
  const images = useDockerImages(host.id, showImages);
  const action = useContainerAction(host.id);
  const rows = useMemo(
    () => mergeStats(containers.data?.items ?? [], stats.data?.items),
    [containers.data, stats.data],
  );

  const run = async (c: ContainerRow, act: ContainerAction) => {
    const danger = act !== "start";
    if (
      danger &&
      !(await confirmAction({
        title: `${t(containerTitle[act])} ${c.name}？`,
        description:
          act === "remove"
            ? t("Data not in a mounted volume is lost.")
            : act === "stop"
              ? t("Services in it stop working.")
              : t("Services in it are unavailable for a moment."),
        confirmLabel: t(containerVerb[act]),
      }))
    )
      return;
    const call = () => action.mutateAsync({ id: c.id, action: act });
    (danger ? withElevation(call) : call()).then(
      () => toast(t("Done")),
      showError,
    );
  };

  return (
    <div className="xc-stack">
      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Containers")}</h2>
          <div className="xc-row monitoring-wrap">
            <label className="monitoring-check">
              <input
                type="checkbox"
                checked={all}
                onChange={(e) => setAll(e.target.checked)}
              />
              <span>{t("Show stopped")}</span>
            </label>
            <button
              className="xc-btn small"
              onClick={() => {
                containers.refetch();
                stats.refetch();
              }}
              disabled={containers.isFetching}
              aria-label={t("Refresh")}
            >
              <RefreshCw
                size={14}
                className={
                  containers.isFetching || stats.isFetching
                    ? "monitoring-spin"
                    : ""
                }
              />
            </button>
          </div>
        </div>
        {containers.isPending ? (
          <Loading />
        ) : containers.isError ? (
          <ErrorState
            error={containers.error}
            onRetry={() => containers.refetch()}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title={all ? t("No containers") : t("No running containers")}
          />
        ) : (
          <div className="xc-table-wrap">
            <table className="xc-table monitoring-table">
              <thead>
                <tr>
                  <th>{t("Name")}</th>
                  <th>{t("State")}</th>
                  <th>{t("Ports")}</th>
                  <th>CPU</th>
                  <th>{t("Memory")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((c) => (
                  <tr key={c.id}>
                    <td className="monitoring-cell-main">
                      <strong>{c.name}</strong>
                      <small className="xc-muted xc-mono">{c.image}</small>
                    </td>
                    <td>
                      <span className={`xc-badge ${containerTone(c.state)}`}>
                        {t(`state.${c.state}`).replace(/^state\./, "")}
                      </span>
                      <small className="xc-muted monitoring-block">
                        {c.status}
                      </small>
                    </td>
                    <td className="xc-mono monitoring-small">
                      {formatPorts(c.ports) || "—"}
                    </td>
                    <td>
                      {c.stats ? `${c.stats.cpuPercent.toFixed(1)}%` : "—"}
                    </td>
                    <td>
                      {c.stats ? (
                        <>
                          {formatBytes(c.stats.memUsage)}
                          {c.stats.memLimit > 0 && (
                            <small className="xc-muted">
                              {" "}
                              / {formatBytes(c.stats.memLimit)}
                            </small>
                          )}
                        </>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="monitoring-cell-actions">
                      {c.state === "running" ? (
                        <>
                          <button
                            className="xc-btn small"
                            disabled={action.isPending}
                            onClick={() => run(c, "restart")}
                            title={t("Restart")}
                          >
                            <RotateCw size={13} />
                          </button>
                          <button
                            className="xc-btn small danger"
                            disabled={action.isPending}
                            onClick={() => run(c, "stop")}
                            title={t("Stop")}
                          >
                            <Square size={12} />
                          </button>
                        </>
                      ) : (
                        <>
                          <button
                            className="xc-btn small"
                            disabled={action.isPending}
                            onClick={() => run(c, "start")}
                            title={t("Start")}
                          >
                            <Play size={13} />
                          </button>
                          <button
                            className="xc-btn small danger"
                            disabled={action.isPending}
                            onClick={() => run(c, "remove")}
                            title={t("Remove")}
                          >
                            <Trash2 size={13} />
                          </button>
                        </>
                      )}
                      <button
                        className="xc-btn small ghost"
                        onClick={() => setLogsFor(c)}
                        title={t("Logs")}
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
      </div>

      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Images")}</h2>
          <button
            className="xc-btn small ghost"
            onClick={() => setShowImages(!showImages)}
          >
            {showImages ? t("Hide") : t("Show")}
          </button>
        </div>
        {showImages &&
          (images.isPending ? (
            <Loading />
          ) : images.isError ? (
            <ErrorState error={images.error} onRetry={() => images.refetch()} />
          ) : images.data.items.length === 0 ? (
            <EmptyState title={t("No images")} />
          ) : (
            <div className="xc-table-wrap">
              <table className="xc-table monitoring-table">
                <thead>
                  <tr>
                    <th>{t("Tag")}</th>
                    <th>{t("Size")}</th>
                    <th>{t("Used by")}</th>
                  </tr>
                </thead>
                <tbody>
                  {images.data.items.map((img) => (
                    <tr key={img.id}>
                      <td className="xc-mono monitoring-small">
                        {img.tags.join(", ") ||
                          img.id.replace("sha256:", "").slice(0, 12)}
                      </td>
                      <td>{formatBytes(img.size)}</td>
                      <td>{img.containers >= 0 ? img.containers : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
      </div>

      <LogsDialog
        hostId={host.id}
        container={logsFor}
        onClose={() => setLogsFor(null)}
      />
    </div>
  );
}

type LogStatus = "connecting" | "following" | "ended" | "error";

/** 跟随容器日志。关闭弹窗就断开。 */
function LogsDialog({
  hostId,
  container,
  onClose,
}: {
  hostId: string;
  container: ContainerRow | null;
  onClose: () => void;
}) {
  const t = useT();
  const [buf, setBuf] = useState(emptyLog);
  const [status, setStatus] = useState<LogStatus>("connecting");
  const [error, setError] = useState("");
  const box = useRef<HTMLPreElement>(null);
  const id = container?.id;

  useEffect(() => {
    if (!id) return;
    setBuf(emptyLog);
    setStatus("connecting");
    setError("");
    const ws = new WebSocket(
      wsUrl(
        `/hosts/${encodeURIComponent(hostId)}/docker/containers/${encodeURIComponent(id)}/logs/follow?tail=300`,
      ),
    );
    let opened = false;
    ws.onopen = () => {
      opened = true;
      setStatus("following");
    };
    ws.onmessage = (event) => {
      if (typeof event.data === "string")
        setBuf((b) => appendLog(b, event.data));
    };
    ws.onclose = (event) => {
      if (event.code === 1000) setStatus("ended");
      else {
        setStatus("error");
        setError(
          event.reason ||
            (opened ? "Connection closed" : "Could not read the logs"),
        );
      }
    };
    return () => ws.close();
  }, [hostId, id]);

  // 新日志到来时滚到底部，除非你往上翻了。
  useEffect(() => {
    const el = box.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 80)
      el.scrollTop = el.scrollHeight;
  }, [buf]);

  const statusText: Record<LogStatus, string> = {
    connecting: "Connecting",
    following: "Following",
    ended: "Log ended",
    error: "Disconnected",
  };
  return (
    <Dialog
      open={!!container}
      onClose={onClose}
      title={`${t("Logs")} · ${container?.name ?? ""}`}
      wide
    >
      <div className="monitoring-detail-bar">
        <span
          className={`xc-badge ${status === "following" ? "ok" : status === "error" ? "danger" : ""}`}
        >
          {t(statusText[status])}
        </span>
        {error && <span className="xc-muted monitoring-small">{t(error)}</span>}
      </div>
      <pre ref={box} className="monitoring-output monitoring-logs">
        {logText(buf) || (status === "following" ? t("No log lines yet") : "")}
      </pre>
      <div className="xc-dialog-actions">
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}
