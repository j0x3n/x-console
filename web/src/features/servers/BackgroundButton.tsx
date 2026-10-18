import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { Bot, Layers, Loader2, SquareTerminal, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { badgeLabel } from "../../lib/navBadges";
import { useCancelTask, useTasks, type Task } from "../coding/api";
import {
  backgroundSessions,
  disconnect,
  isBusy,
  useTerminals,
  type TermInfo,
} from "./terminal/store";
import "./terminal/i18n";
import "./terminal/background.css";

type T = ReturnType<typeof useT>;
const activeStatuses: Task["status"][] = ["queued", "running"];

/** “12 分钟”“1 小时 5 分钟”。 */
export function sinceLabel(t: T, ms: number): string {
  const minutes = Math.floor(Math.max(0, ms) / 60000);
  if (minutes < 1) return t("Under a minute");
  if (minutes < 60) return t("{n} min").replace("{n}", String(minutes));
  return t("{h} h {m} min")
    .replace("{h}", String(Math.floor(minutes / 60)))
    .replace("{m}", String(minutes % 60));
}

function useNow(ms: number, on: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!on) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), ms);
    return () => clearInterval(id);
  }, [ms, on]);
  return now;
}

/**
 * B249：顶栏的“后台”按钮。数量是留在后台的终端加正在跑的 Agent 任务，0 时不显示。
 * 点开列出每一条：点行跳回去，点 × 断开终端或取消任务。手机上从底部弹出。
 */
export default function BackgroundButton() {
  const t = useT();
  const navigate = useNavigate();
  const sessions = useTerminals((s) => s.sessions);
  const terms = backgroundSessions(sessions);
  // 靠 coding_task 事件刷新，不另外轮询
  const tasksQuery = useTasks(activeStatuses);
  const tasks = (tasksQuery.data ?? []).filter((x) =>
    activeStatuses.includes(x.status),
  );
  const cancel = useCancelTask();
  const count = terms.length + tasks.length;
  const [open, setOpen] = useState(false);
  const now = useNow(5000, open);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (count === 0) setOpen(false);
  }, [count]);
  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) =>
      event.key === "Escape" && setOpen(false);
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (count === 0) return null;

  const go = (path: string) => {
    setOpen(false);
    navigate(path);
  };
  const closeTerm = async (s: TermInfo) => {
    const ok = await confirmAction({
      title: t("Disconnect the terminal on {name}?").replace(
        "{name}",
        s.hostName,
      ),
      description: t("Running commands stop."),
      confirmLabel: t("Disconnect"),
    });
    if (ok) disconnect(s.hostId);
  };
  const cancelTask = async (task: Task) => {
    const ok = await confirmAction({
      title: t("Cancel task {name}?").replace("{name}", taskName(task)),
      description:
        task.status === "running"
          ? t("A running task stops and its changes are thrown away.")
          : undefined,
      confirmLabel: t("Cancel task"),
    });
    if (!ok) return;
    cancel.mutate(task.id, {
      onSuccess: () => toast(t("Canceled")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };

  const label = t("{n} in the background").replace("{n}", String(count));
  const mark = badgeLabel(count);
  return (
    <div className="bg-wrap" ref={ref}>
      <button
        type="button"
        className={`icon-button bg-button${open ? " on" : ""}`}
        aria-label={label}
        title={label}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <Layers size={16} />
        <span className={`bg-count${mark ? "" : " dot"}`}>{mark}</span>
      </button>
      {open && (
        <>
          <div className="bg-backdrop" onClick={() => setOpen(false)} />
          <div
            className="bg-panel"
            role="dialog"
            aria-label={t("Background work")}
          >
            <div className="bg-panel-title">{t("Background work")}</div>
            {terms.length > 0 && (
              <section>
                <h3>{t("Terminal")}</h3>
                {terms.map((s) => {
                  const busy = isBusy(s.hostId, now);
                  return (
                    <div className="bg-row" key={s.hostId}>
                      <button
                        type="button"
                        className="bg-row-main"
                        onClick={() => go(s.path)}
                      >
                        {busy ? (
                          <Loader2
                            size={14}
                            className="bg-spin"
                            aria-label={t("Command running")}
                          />
                        ) : (
                          <SquareTerminal size={14} aria-hidden />
                        )}
                        <span className="bg-row-text">
                          <strong>{s.hostName}</strong>
                          <small>
                            {s.title || t("Terminal")} ·{" "}
                            {sinceLabel(t, now - (s.parkedAt ?? now))}
                          </small>
                        </span>
                      </button>
                      <button
                        type="button"
                        className="icon-button bg-row-x"
                        aria-label={t(
                          "Disconnect the terminal on {name}?",
                        ).replace("{name}", s.hostName)}
                        title={t("Disconnect")}
                        onClick={() => void closeTerm(s)}
                      >
                        <X size={14} />
                      </button>
                    </div>
                  );
                })}
              </section>
            )}
            {tasks.length > 0 && (
              <section>
                <h3>{t("Agent tasks")}</h3>
                {tasks.map((task) => (
                  <div className="bg-row" key={task.id}>
                    <button
                      type="button"
                      className="bg-row-main"
                      onClick={() => go(`/coding/${task.id}`)}
                    >
                      {task.status === "running" ? (
                        <Loader2 size={14} className="bg-spin" aria-hidden />
                      ) : (
                        <Bot size={14} aria-hidden />
                      )}
                      <span className="bg-row-text">
                        <strong>{taskName(task)}</strong>
                        <small>
                          {task.repoName} ·{" "}
                          {t(task.status === "running" ? "Running" : "Queued")}
                          {" · "}
                          {sinceLabel(
                            t,
                            now -
                              new Date(
                                task.startedAt ?? task.createdAt,
                              ).getTime(),
                          )}
                        </small>
                      </span>
                    </button>
                    <button
                      type="button"
                      className="icon-button bg-row-x"
                      aria-label={t("Cancel task {name}?").replace(
                        "{name}",
                        taskName(task),
                      )}
                      title={t("Cancel task")}
                      disabled={cancel.isPending}
                      onClick={() => void cancelTask(task)}
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </section>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function taskName(task: Pick<Task, "title" | "id">) {
  return task.title || `#${task.id}`;
}
