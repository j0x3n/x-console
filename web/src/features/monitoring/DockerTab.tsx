import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronRight,
  Eraser,
  FileText,
  Play,
  RefreshCw,
  RotateCw,
  Square,
  Trash2,
} from "lucide-react";
import LogViewer from "../../components/log/LogViewer";
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
  usePruneImages,
  useRemoveImage,
  useContainerAction,
  useContainers,
  useDockerImages,
  useDockerStats,
  type ContainerAction,
  type DockerImage,
} from "./api";
import { showError } from "./components/common";
import {
  appendLog,
  appendLogLines,
  containerTone,
  emptyLog,
  formatPorts,
  mergeStats,
  parseLogFrame,
  sortContainers,
  type ContainerRow,
  type ContainerSort,
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
  // 镜像默认展开（B28）。
  const [showImages, setShowImages] = useState(true);
  const [sort, setSort] = useState<ContainerSort>("name");
  const [logsFor, setLogsFor] = useState<ContainerRow | null>(null);
  const containers = useContainers(host.id, all);
  const stats = useDockerStats(host.id);
  const images = useDockerImages(host.id, showImages);
  const action = useContainerAction(host.id);
  const removeImage = useRemoveImage(host.id);
  const prune = usePruneImages(host.id);
  const rows = useMemo(
    () =>
      sortContainers(
        mergeStats(containers.data?.items ?? [], stats.data?.items),
        sort,
      ),
    [containers.data, stats.data, sort],
  );
  const imageName = (img: DockerImage) =>
    img.tags.join(", ") || img.id.replace("sha256:", "").slice(0, 12);
  const usersOf = (img: DockerImage) =>
    (containers.data?.items ?? [])
      .filter((c) => img.tags.includes(c.image) || c.image === img.id)
      .map((c) => c.name);
  const onRemoveImage = async (img: DockerImage) => {
    if (
      !(await confirmAction({
        title: `${t("Delete image")} ${imageName(img)}？`,
        description: `${t("Frees")} ${formatBytes(img.size)}。`,
      }))
    )
      return;
    removeImage.mutate(img.id, {
      onSuccess: () => toast(t("Deleted")),
      onError: showError,
    });
  };
  const unused = (images.data?.items ?? []).filter((i) => i.containers === 0);
  const onPrune = async () => {
    const bytes = unused.reduce((sum, i) => sum + i.size, 0);
    if (
      !(await confirmAction({
        title: `${t("Delete")} ${unused.length} ${t("unused images")}？`,
        description: `${t("Frees about")} ${formatBytes(bytes)}。`,
        confirmLabel: t("Clean up"),
      }))
    )
      return;
    prune.mutate(undefined, {
      onSuccess: (r) =>
        toast(
          `${t("Deleted")} ${r.deleted} · ${formatBytes(r.spaceReclaimed)}`,
        ),
      onError: showError,
    });
  };
  const sortHead = (key: ContainerSort, label: string) => (
    <th>
      <button
        className={`monitoring-sort${sort === key ? " active" : ""}`}
        aria-pressed={sort === key}
        onClick={() => setSort(key)}
      >
        {label}
      </button>
    </th>
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
            <select
              className="xc-select monitoring-sort-select"
              aria-label={t("Sort by")}
              value={sort}
              onChange={(e) => setSort(e.target.value as ContainerSort)}
            >
              <option value="name">{t("Name")}</option>
              <option value="state">{t("State")}</option>
              <option value="cpu">CPU</option>
              <option value="mem">{t("Memory")}</option>
            </select>
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
                  {sortHead("name", t("Name"))}
                  {sortHead("state", t("State"))}
                  <th>{t("Ports")}</th>
                  {sortHead("cpu", "CPU")}
                  {sortHead("mem", t("Memory"))}
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
          <button
            className="monitoring-collapse"
            aria-expanded={showImages}
            onClick={() => setShowImages(!showImages)}
          >
            <ChevronRight size={14} />
            <h2>{t("Images")}</h2>
            {images.data && (
              <span className="xc-muted">{images.data.items.length}</span>
            )}
          </button>
          {showImages && unused.length > 0 && (
            <button
              className="xc-btn small"
              disabled={prune.isPending}
              onClick={onPrune}
            >
              <Eraser size={13} /> {t("Clean up unused images")}
            </button>
          )}
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
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {images.data.items.map((img) => {
                    const users = usersOf(img);
                    const inUse = img.containers > 0 || users.length > 0;
                    return (
                      <tr key={img.id}>
                        <td className="xc-mono monitoring-small">
                          {imageName(img)}
                        </td>
                        <td>{formatBytes(img.size)}</td>
                        <td title={users.join(", ")}>
                          {img.containers >= 0 ? img.containers : "—"}
                        </td>
                        <td className="monitoring-cell-actions">
                          <button
                            className="xc-btn small danger"
                            disabled={inUse || removeImage.isPending}
                            title={
                              inUse
                                ? `${t("Delete the containers using it first")}${users.length ? `：${users.join(", ")}` : ""}`
                                : t("Delete image")
                            }
                            aria-label={`${t("Delete image")} ${imageName(img)}`}
                            onClick={() => onRemoveImage(img)}
                          >
                            <Trash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    );
                  })}
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

/** 跟随容器日志（B28 起用公共的 LogViewer，可以按级别、输出、关键字过滤）。 */
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
  const [structured, setStructured] = useState(false);
  const [status, setStatus] = useState<LogStatus>("connecting");
  const [error, setError] = useState("");
  const id = container?.id;

  useEffect(() => {
    if (!id) return;
    setBuf(emptyLog);
    setStructured(false);
    setStatus("connecting");
    setError("");
    const ws = new WebSocket(
      wsUrl(
        `/hosts/${encodeURIComponent(hostId)}/docker/containers/${encodeURIComponent(id)}/logs/follow?tail=500&format=json`,
      ),
    );
    let opened = false;
    ws.onopen = () => {
      opened = true;
      setStatus("following");
    };
    ws.onmessage = (event) => {
      if (typeof event.data !== "string") return;
      const items = parseLogFrame(event.data);
      if (items) {
        setStructured(true);
        setBuf((b) => appendLogLines(b, items));
      } else setBuf((b) => appendLog(b, event.data));
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

  const lines = buf.partial
    ? [...buf.lines, { id: buf.nextId, text: buf.partial }]
    : buf.lines;
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
      <LogViewer
        lines={lines}
        hasStream={structured}
        height="min(60vh, 520px)"
        empty={status === "following" ? t("No log lines yet") : undefined}
        toolbarEnd={
          <span
            className={`xc-badge ${status === "following" ? "ok" : status === "error" ? "danger" : ""}`}
            title={error ? t(error) : undefined}
          >
            {t(statusText[status])}
          </span>
        }
      />
      <div className="xc-dialog-actions">
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}
