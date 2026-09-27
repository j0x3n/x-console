import { CheckCircle2, CircleAlert, X } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import { useUploads } from "../upload";

/** 右下角的上传进度，每个文件一条。全部结束后可以关掉。 */
export default function UploadPanel() {
  const t = useT();
  const tasks = useUploads((s) => s.tasks);
  const clear = useUploads((s) => s.clearFinished);
  if (tasks.length === 0) return null;
  const running = tasks.filter(
    (x) => x.state === "queued" || x.state === "uploading",
  ).length;
  return (
    <section className="drive-uploads" aria-label={t("Uploads")}>
      <header>
        <strong>
          {running > 0 ? `${t("Uploading")} ${running}` : t("Uploads finished")}
        </strong>
        {running === 0 && (
          <button
            type="button"
            className="xc-btn ghost small"
            aria-label={t("Close")}
            onClick={clear}
          >
            <X size={14} />
          </button>
        )}
      </header>
      <ul>
        {tasks.map((task) => {
          const pct = task.size
            ? Math.round((task.loaded / task.size) * 100)
            : 100;
          return (
            <li key={task.id} className={task.state}>
              <div className="drive-upload-row">
                <span className="drive-upload-name">{task.name}</span>
                {task.state === "done" ? (
                  <CheckCircle2 size={14} className="ok" />
                ) : task.state === "failed" ? (
                  <CircleAlert size={14} className="danger" />
                ) : (
                  <small>
                    {task.state === "queued" ? t("Waiting") : `${pct}%`}
                  </small>
                )}
              </div>
              {task.state === "failed" ? (
                <small className="danger">{task.error}</small>
              ) : (
                <div
                  className="drive-bar"
                  role="progressbar"
                  aria-valuenow={pct}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={task.name}
                >
                  <i
                    style={{ width: `${task.state === "done" ? 100 : pct}%` }}
                  />
                </div>
              )}
              <small className="drive-muted">{formatBytes(task.size)}</small>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
