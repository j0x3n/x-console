import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  Camera,
  Hand,
  LogIn,
  Pause,
  Pencil,
  Play,
  RefreshCw,
  Sparkles,
  Trash2,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { useAgents } from "../../api/core";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageActions from "../../components/layout/PageActions";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime, relativeTime } from "../../lib/time";
import { usePageCrumb } from "../../stores/page-title";
import {
  shotUrl,
  useAnswerWatcher,
  useDeleteWatcher,
  useRunWatcher,
  useUpdateWatcher,
  useWatcher,
  type Answer,
  type RunKind,
  type WatcherDetail,
  type WatcherRun,
} from "./api";
import { minutesText, money, STATUS, xLabel } from "./lib";
import WatcherDialog from "./WatcherDialog";
import "./i18n";
import "./watchers.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

const RUN_KIND: Record<WatcherRun["kind"], string> = {
  check: "检查",
  explore: "AI 摸索",
  manual: "手动登录",
  keepalive: "保活",
};

const OUTCOME: Record<WatcherRun["outcome"], { label: string; tone: string }> =
  {
    running: { label: "进行中", tone: "info" },
    same: { label: "没变化", tone: "" },
    changed: { label: "有变化", tone: "accent" },
    ok: { label: "完成", tone: "ok" },
    failed: { label: "失败", tone: "danger" },
    cancelled: { label: "取消", tone: "" },
  };

/** 一个网站自动化任务的详情：当前值、等你处理的事、变化历史、检查记录。 */
export default function WatcherPage() {
  const t = useT();
  const id = Number(useParams().watcherId);
  const watcher = useWatcher(id);
  usePageCrumb(watcher.data?.name ?? "", [
    { label: t("Web watchers"), to: "/monitoring/watchers" },
  ]);
  if (watcher.isPending) return <Loading />;
  if (watcher.isError)
    return (
      <div className="xc-page">
        <ErrorState error={watcher.error} onRetry={() => watcher.refetch()} />
      </div>
    );
  return <Detail w={watcher.data} />;
}

function Detail({ w }: { w: WatcherDetail }) {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const run = useRunWatcher();
  const update = useUpdateWatcher();
  const remove = useDeleteWatcher();
  const [editing, setEditing] = useState(false);
  const agents = useAgents();
  const st = STATUS[w.status];
  const busy =
    w.status === "checking" ||
    w.status === "exploring" ||
    w.status === "waiting";

  const start = (kind: RunKind, message: string) =>
    run.mutate(
      { id: w.id, kind },
      {
        onSuccess: () => {
          toast(message);
          if (kind === "manual") navigate(`/monitoring/watchers/${w.id}/live`);
        },
        onError: fail,
      },
    );
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete watcher")} ${w.name}？`,
      description: "检查记录、截图和保存的登录状态会一起删掉。",
      confirmLabel: t("Delete"),
    });
    if (ok)
      remove.mutate(w.id, {
        onSuccess: () => {
          toast(t("Deleted"));
          navigate("/monitoring/watchers");
        },
        onError: fail,
      });
  };

  return (
    <div className="xc-page">
      <PageActions>
        <button
          className="xc-btn"
          title={t("Edit")}
          aria-label={t("Edit")}
          onClick={() => setEditing(true)}
        >
          <Pencil size={15} />
        </button>
        <button
          className="xc-btn primary"
          title={t("Check now")}
          disabled={busy || !w.enabled}
          onClick={() => start("check", "开始检查")}
        >
          <RefreshCw size={15} /> {t("Check now")}
        </button>
      </PageActions>
      {w.pending && <PendingCard w={w} />}
      <div className="watchers-detail">
        <div className="watchers-main">
          <section className="xc-card watchers-value-card">
            <div className="xc-card-head">
              <h2>{w.kind === "x" ? t("Latest post") : t("Current value")}</h2>
              <span className={`xc-badge ${st.tone}`}>{st.label}</span>
            </div>
            <p className="watchers-value">
              {w.currentValue || <span className="xc-muted">还没读到</span>}
            </p>
            {w.statusDetail && w.status !== "ok" && (
              <p
                className={`watchers-detail-note${w.status === "error" ? " danger" : ""}`}
              >
                {w.statusDetail}
              </p>
            )}
            {w.kind === "web" && w.targetNote && (
              <p className="xc-muted watchers-small">位置：{w.targetNote}</p>
            )}
          </section>
          <section className="xc-card">
            <div className="xc-card-head">
              <h2>{t("Value changes")}</h2>
              <span className="xc-muted">{w.changes.length}</span>
            </div>
            {w.changes.length === 0 ? (
              <p className="xc-muted watchers-small">还没有变化。</p>
            ) : (
              <ul className="watchers-changes">
                {w.changes.map((c) => (
                  <li key={c.id}>
                    <div className="watchers-change-head">
                      <strong>{c.summary}</strong>
                      {!c.important && <span className="xc-badge">不重要</span>}
                      <span className="xc-muted">
                        {formatDate(c.createdAt, language)}{" "}
                        {formatTime(c.createdAt, language)}
                      </span>
                      {c.shotId != null && (
                        <a
                          className="xc-btn ghost small"
                          href={shotUrl(c.shotId)}
                          target="_blank"
                          rel="noreferrer"
                          title={t("Screenshot")}
                          aria-label={t("Screenshot")}
                        >
                          <Camera size={14} />
                        </a>
                      )}
                    </div>
                    <div className="watchers-change-values">
                      <span className="old">{c.oldValue}</span>
                      <span className="arrow">→</span>
                      <span>{c.newValue}</span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section className="xc-card">
            <div className="xc-card-head">
              <h2>{t("Check history")}</h2>
            </div>
            {w.runs.length === 0 ? (
              <p className="xc-muted watchers-small">还没检查过。</p>
            ) : (
              <div className="watchers-runs">
                {w.runs.map((r, i) => (
                  <RunItem key={r.id} r={r} open={i === 0} />
                ))}
              </div>
            )}
          </section>
        </div>
        <aside className="watchers-side">
          <section className="xc-card watchers-actions">
            {w.kind === "web" && (
              <>
                <button
                  className="xc-btn"
                  disabled={busy}
                  onClick={() => start("explore", "AI 开始重新摸索")}
                >
                  <Sparkles size={15} /> {t("Explore again")}
                </button>
                <button
                  className="xc-btn"
                  disabled={busy}
                  onClick={() => start("manual", "浏览器已打开")}
                >
                  <LogIn size={15} /> {t("Log in by hand")}
                </button>
              </>
            )}
            <button
              className="xc-btn"
              onClick={() =>
                update.mutate(
                  { id: w.id, body: { enabled: !w.enabled } },
                  { onError: fail },
                )
              }
            >
              {w.enabled ? <Pause size={15} /> : <Play size={15} />}{" "}
              {w.enabled ? t("Pause") : t("Resume")}
            </button>
            <button className="xc-btn danger" onClick={onDelete}>
              <Trash2 size={15} /> {t("Delete")}
            </button>
          </section>
          <section className="xc-card">
            <dl className="watchers-meta">
              <dt>{t("Address")}</dt>
              <dd>
                <a href={w.url} target="_blank" rel="noreferrer">
                  {w.kind === "x" ? xLabel(w.url) : w.url}
                </a>
              </dd>
              {w.kind === "x" ? (
                <>
                  <dt>{t("Only tell me about")}</dt>
                  <dd>{w.goal || "全部提醒"}</dd>
                  <dt>{t("Check every")}</dt>
                  <dd>{minutesText(w.intervalMinutes)}</dd>
                </>
              ) : (
                <>
                  <dt>{t("What AI should do")}</dt>
                  <dd>{w.goal || "没写，AI 自己找要盯的内容"}</dd>
                  <dt>{t("Check every")}</dt>
                  <dd>{minutesText(w.intervalMinutes)}</dd>
                  <dt>{t("Watch mode")}</dt>
                  <dd>
                    {w.mode === "ai"
                      ? "每次都让 AI 看"
                      : "AI 记下步骤，平时回放"}
                  </dd>
                  <dt>{t("Login")}</dt>
                  <dd>{w.hasLogin ? "已存用户名和密码" : "没有存"}</dd>
                </>
              )}
              {w.keepaliveMinutes > 0 && (
                <>
                  <dt>{t("Keep login alive")}</dt>
                  <dd>每 {minutesText(w.keepaliveMinutes)}</dd>
                </>
              )}
              {(w.hasProxy || w.egressAgentId) && (
                <>
                  <dt>{t("Go online via")}</dt>
                  <dd>
                    {w.egressAgentId
                      ? (agents.data?.find((a) => a.id === w.egressAgentId)
                          ?.name ?? "已删除的机器")
                      : "代理"}
                  </dd>
                </>
              )}
              <dt>{t("Last check")}</dt>
              <dd>
                {w.lastCheckedAt
                  ? relativeTime(w.lastCheckedAt, language)
                  : "—"}
              </dd>
              <dt>{t("Next check")}</dt>
              <dd>
                {w.enabled && w.nextCheckAt
                  ? relativeTime(w.nextCheckAt, language)
                  : "—"}
              </dd>
              <dt>{t("AI cost, 30 days")}</dt>
              <dd>{money(w.spent30d)}</dd>
            </dl>
          </section>
          {w.recipe.length > 0 && (
            <section className="xc-card">
              <div className="xc-card-head">
                <h2>{t("Recorded steps")}</h2>
              </div>
              <ol className="watchers-recipe">
                {w.recipe.map((s, i) => (
                  <li key={i}>{s.text}</li>
                ))}
              </ol>
            </section>
          )}
        </aside>
      </div>
      <WatcherDialog
        open={editing}
        watcher={w}
        onClose={() => setEditing(false)}
      />
    </div>
  );
}

/** 任务在等你：验证码、扫码、批准一个按钮。 */
export function PendingCard({
  w,
  onLivePage = false,
}: {
  w: WatcherDetail;
  onLivePage?: boolean;
}) {
  const t = useT();
  const answer = useAnswerWatcher();
  const p = w.pending!;
  const send = (a: Answer) =>
    answer.mutate({ id: w.id, answer: a }, { onError: fail });
  return (
    <section className="xc-card watchers-pending">
      <Hand size={17} />
      <div className="watchers-pending-text">
        <strong>{t("Needs you")}</strong>
        <span>{p.message}</span>
      </div>
      <div className="watchers-pending-actions">
        {p.kind === "approve" ? (
          <>
            <button className="xc-btn" onClick={() => send("deny")}>
              {t("Refuse click")}
            </button>
            <button className="xc-btn primary" onClick={() => send("approve")}>
              {t("Allow click")}
            </button>
          </>
        ) : (
          <>
            {p.live && !onLivePage && (
              <Link
                className="xc-btn primary"
                to={`/monitoring/watchers/${w.id}/live`}
              >
                {t("Open the browser")}
              </Link>
            )}
            <button className="xc-btn" onClick={() => send("cancel")}>
              {t("Skip this time")}
            </button>
          </>
        )}
      </div>
    </section>
  );
}

function RunItem({ r, open }: { r: WatcherRun; open: boolean }) {
  const language = useLanguage();
  const o = OUTCOME[r.outcome];
  return (
    <details className="watchers-run" open={open}>
      <summary>
        <span className="watchers-run-kind">{RUN_KIND[r.kind]}</span>
        <span className={`xc-badge ${o.tone}`}>{o.label}</span>
        <span className="watchers-run-detail">{r.detail}</span>
        <span className="xc-muted">{relativeTime(r.startedAt, language)}</span>
      </summary>
      <ol className="watchers-steps">
        {r.steps.map((s, i) => (
          <li key={i}>
            <span>{s.text}</span>
            {s.shotId != null && (
              <a href={shotUrl(s.shotId)} target="_blank" rel="noreferrer">
                <img src={shotUrl(s.shotId)} alt="截图" loading="lazy" />
              </a>
            )}
          </li>
        ))}
      </ol>
      {r.tokens > 0 && (
        <p className="xc-muted watchers-small">
          用了 {r.tokens.toLocaleString()} 个 token
        </p>
      )}
    </details>
  );
}
