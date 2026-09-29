import { useEffect, useRef, useState } from "react";
import { CheckCircle2, CircleAlert, CircleSlash, X } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes } from "../../../lib/time";
import {
  useCancelTask,
  useDriveTasks,
  useTaskEvents,
  type DriveTask,
} from "../api";
import { taskPercent } from "../logic";

/**
 * 右下角的后台任务（B31）：复制、移动、压缩、解压。
 * 每个任务一条进度，跑着的可以取消。全部结束后可以关掉。
 * 结束时弹一条提示。
 */
export default function TaskPanel({
  onOpenFolder,
}: {
  onOpenFolder: (folderId: number) => void;
}) {
  const t = useT();
  useTaskEvents();
  const tasks = useDriveTasks();
  const cancel = useCancelTask();
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const seen = useRef(new Map<string, DriveTask["state"]>());
  // 这次打开页面后见过“进行中”的任务。之前就结束的任务不显示。
  const [watched, setWatched] = useState<Set<string>>(new Set());

  // 从“进行中”变成结束时提示一次。页面打开前就结束的任务不提示。
  useEffect(() => {
    const running = (tasks.data ?? []).filter(
      (x) => x.state === "running" && !watched.has(x.id),
    );
    if (running.length)
      setWatched((cur) => new Set([...cur, ...running.map((x) => x.id)]));
    for (const task of tasks.data ?? []) {
      const before = seen.current.get(task.id);
      seen.current.set(task.id, task.state);
      if (before !== "running" || task.state === "running") continue;
      if (task.state === "done")
        toast(
          task.skipped
            ? `${task.title}：${t("Done")}，${t("skipped")} ${task.skipped}`
            : `${task.title}：${t("Done")}`,
        );
      else if (task.state === "failed")
        toast({ message: `${task.title}：${task.error ?? ""}`, tone: "error" });
    }
  }, [tasks.data]);

  const list = (tasks.data ?? []).filter(
    (x) => !dismissed.has(x.id) && (x.state === "running" || watched.has(x.id)),
  );
  if (list.length === 0) return null;
  const running = list.filter((x) => x.state === "running").length;

  return (
    <section
      className="drive-uploads drive-tasks"
      aria-label={t("Background tasks")}
    >
      <header>
        <strong>
          {running > 0
            ? `${t("Tasks running")} ${running}`
            : t("Background tasks finished")}
        </strong>
        {running === 0 && (
          <button
            type="button"
            className="xc-btn ghost small"
            aria-label={t("Close")}
            onClick={() => setDismissed(new Set(list.map((x) => x.id)))}
          >
            <X size={14} />
          </button>
        )}
      </header>
      <ul>
        {list.map((task) => {
          const pct = taskPercent(task);
          return (
            <li key={task.id} className={task.state}>
              <div className="drive-upload-row">
                <span className="drive-upload-name" title={task.title}>
                  {task.title}
                </span>
                {task.state === "running" ? (
                  <>
                    <small>{pct == null ? t("Counting") : `${pct}%`}</small>
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-label={`${t("Cancel")} ${task.title}`}
                      title={t("Cancel")}
                      disabled={cancel.isPending}
                      onClick={() => cancel.mutate(task.id)}
                    >
                      <X size={13} />
                    </button>
                  </>
                ) : task.state === "done" ? (
                  <CheckCircle2 size={14} className="ok" />
                ) : task.state === "canceled" ? (
                  <CircleSlash size={14} className="drive-muted" />
                ) : (
                  <CircleAlert size={14} className="danger" />
                )}
              </div>
              {task.state === "failed" ? (
                <small className="danger">{task.error}</small>
              ) : task.state === "canceled" ? (
                <small className="drive-muted">{t("Canceled")}</small>
              ) : (
                <div
                  className="drive-bar"
                  role="progressbar"
                  aria-valuenow={pct ?? 0}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={task.title}
                >
                  <i
                    style={{
                      width: `${task.state === "done" ? 100 : (pct ?? 0)}%`,
                    }}
                  />
                </div>
              )}
              <small className="drive-muted drive-task-meta">
                {task.state === "running" && task.current ? (
                  <span className="drive-upload-name">{task.current}</span>
                ) : (
                  <span>
                    {task.totalItems > 0 &&
                      `${task.doneItems} / ${task.totalItems} ${t("items")}`}
                    {task.totalBytes > 0 &&
                      ` · ${formatBytes(task.doneBytes)} / ${formatBytes(task.totalBytes)}`}
                  </span>
                )}
                {task.state === "done" && task.targetId != null && (
                  <button
                    type="button"
                    className="drive-link"
                    onClick={() => onOpenFolder(task.targetId ?? 0)}
                  >
                    {t("Open folder")}
                  </button>
                )}
              </small>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
