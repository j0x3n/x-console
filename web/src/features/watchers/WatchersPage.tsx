import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Plus, ScanEye, Settings2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useDeleteWatcher,
  useRunWatcher,
  useUpdateWatcher,
  useWatchers,
  useWatchersStatus,
  type Watcher,
} from "./api";
import { emptyValue, money, needsUser, sortWatchers, STATUS } from "./lib";
import SettingsDialog from "./SettingsDialog";
import WatcherDialog from "./WatcherDialog";
import "./i18n";
import "./watchers.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 网站自动化（B206）列表。放在监控的二级菜单下。 */
export default function WatchersPage() {
  const t = useT();
  const navigate = useNavigate();
  const list = useWatchers();
  const status = useWatchersStatus();
  const [params, setParams] = useSearchParams();
  const creating = params.get("new") === "1";
  const setCreating = (on: boolean) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p);
        if (on) next.set("new", "1");
        else next.delete("new");
        return next;
      },
      { replace: true },
    );
  const [settingsOpen, setSettingsOpen] = useState(false);
  const watchers = sortWatchers(list.data ?? []);
  const waiting = watchers.filter(needsUser).length;
  const failed = watchers.filter((w) => w.status === "error").length;

  const aside = (
    <>
      <button
        className="xc-btn"
        title={t("Watcher settings")}
        aria-label={t("Watcher settings")}
        onClick={() => setSettingsOpen(true)}
      >
        <Settings2 size={15} />
      </button>
      <button
        className="xc-btn primary"
        title={t("New watcher")}
        onClick={() => setCreating(true)}
      >
        <Plus size={15} /> {t("New watcher")}
      </button>
    </>
  );

  let content;
  if (list.isPending) content = <Loading />;
  else if (list.isError && isNotLive(list.error))
    content = <NotLive name={t("Web watchers")} icon={<ScanEye size={28} />} />;
  else if (list.isError)
    content = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else
    content = (
      <>
        <StatStrip label={t("Web watchers")}>
          <StatCard
            label={t("Watchers")}
            value={watchers.length}
            foot={
              failed > 0
                ? `${failed} 个出错`
                : watchers.length
                  ? "都在正常运行"
                  : "还没有任务"
            }
            tone={failed > 0 ? "danger" : undefined}
          />
          <StatCard
            label={t("Needs you")}
            value={waiting}
            tone={waiting > 0 ? "warn" : undefined}
            foot={waiting > 0 ? "验证码、登录或批准" : "没有要你处理的"}
          />
          <StatCard
            label={t("AI cost today")}
            value={money(status.data?.spentToday)}
            foot={
              status.data
                ? `上限 ${money(status.data.budgetUsd)}`
                : t("Loading")
            }
          />
          <StatCard
            label={t("Browser")}
            value={!status.data ? "—" : status.data.browser ? "正常" : "没启动"}
            tone={
              status.data &&
              !status.data.browser &&
              watchers.some((w) => w.kind === "web")
                ? "danger"
                : undefined
            }
            foot={status.data?.ai === false ? "没配 AI 模型" : "AI 已配置"}
          />
        </StatStrip>
        {status.data &&
          !status.data.browser &&
          watchers.some((w) => w.kind === "web") && <BrowserHelp />}
        {status.data &&
          !status.data.x &&
          watchers.some((w) => w.kind === "x") && <XHelp />}
        {watchers.length === 0 ? (
          <EmptyState title={t("No watchers yet")} icon={<ScanEye size={26} />}>
            <span>
              给 AI 一个网址，它会登录，自己找到要盯的内容。变了就通知你。
            </span>
            <button
              className="xc-btn primary"
              onClick={() => setCreating(true)}
            >
              {t("New watcher")}
            </button>
          </EmptyState>
        ) : (
          <div className="watchers-grid">
            {watchers.map((w) => (
              <WatcherCard key={w.id} w={w} />
            ))}
          </div>
        )}
      </>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Web watchers")}
        subtitle={waiting > 0 ? `${waiting} 个等你处理` : undefined}
        aside={aside}
      />
      {content}
      <WatcherDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(w) => navigate(`/monitoring/watchers/${w.id}`)}
      />
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

function BrowserHelp() {
  return (
    <div className="xc-card watchers-help">
      <strong>服务器上的浏览器没有启动</strong>
      <p>
        在服务器的 <code>~/x-console/.env</code> 里加一行{" "}
        <code>COMPOSE_PROFILES=browser</code>（已经有 caddy 的写成{" "}
        <code>caddy,browser</code>），再部署一次。浏览器要多占约 1 GB 内存。
      </p>
    </div>
  );
}

function XHelp() {
  return (
    <div className="xc-card watchers-help">
      <strong>X 的登录 Cookie 没填或者过期了</strong>
      <p>
        盯 X 用的是稍后读里存的 Cookie。去 设置 → 稍后读 填 auth_token 和
        ct0，建议用小号。
      </p>
    </div>
  );
}

/** B234：一个任务一张卡片，直接显示最新结果和最近一次变化。 */
function WatcherCard({ w }: { w: Watcher }) {
  const t = useT();
  const language = useLanguage();
  const run = useRunWatcher();
  const update = useUpdateWatcher();
  const remove = useDeleteWatcher();
  const st = STATUS[w.status];
  const busy = w.status === "checking" || w.status === "exploring";
  const onDelete = async () => {
    const ok = await confirmAction({
      title: `${t("Delete watcher")} ${w.name}？`,
      description: "检查记录、截图和保存的登录状态会一起删掉。",
      confirmLabel: t("Delete"),
    });
    if (ok)
      remove.mutate(w.id, {
        onSuccess: () => toast(t("Deleted")),
        onError: fail,
      });
  };
  const to = `/monitoring/watchers/${w.id}`;
  return (
    <div className={`xc-card watchers-card${needsUser(w) ? " attention" : ""}`}>
      <div className="watchers-card-head">
        <Link to={to} className="watchers-card-name">
          {w.kind === "x" && <span className="watchers-x">X</span>}
          {w.name}
        </Link>
        <span className={`xc-badge ${st.tone}`}>{st.label}</span>
      </div>
      <Link
        to={to}
        className={`watchers-card-value${w.currentValue ? "" : " empty"}`}
        title={w.currentValue || undefined}
      >
        {w.currentValue || emptyValue(w)}
      </Link>
      {w.currentValue && w.statusDetail && w.status !== "ok" && (
        <p
          className={`watchers-card-note${w.status === "error" ? " danger" : ""}`}
        >
          {w.statusDetail}
        </p>
      )}
      {w.lastChange && (
        <p className="watchers-card-change">
          <span className="watchers-card-change-text">
            {w.lastChange.summary}
          </span>
          <span className="xc-muted">
            {relativeTime(w.lastChange.createdAt, language)}
          </span>
        </p>
      )}
      <div className="watchers-card-foot">
        <span className="xc-muted">
          {w.lastCheckedAt
            ? `${t("Last check")} ${relativeTime(w.lastCheckedAt, language)}`
            : "还没检查过"}
        </span>
        {w.pending?.live ? (
          <Link
            className="xc-btn small primary"
            to={`/monitoring/watchers/${w.id}/live`}
          >
            {t("Handle it")}
          </Link>
        ) : (
          <button
            className="xc-btn small"
            disabled={busy || !w.enabled}
            onClick={() =>
              run.mutate(
                { id: w.id, kind: "check" },
                { onSuccess: () => toast("开始检查"), onError: fail },
              )
            }
          >
            {t("Check now")}
          </button>
        )}
        <MoreMenu
          label={t("More")}
          items={[
            {
              key: "toggle",
              label: w.enabled ? t("Pause") : t("Resume"),
              onSelect: () =>
                update.mutate(
                  { id: w.id, body: { enabled: !w.enabled } },
                  { onError: fail },
                ),
            },
            {
              key: "delete",
              label: t("Delete"),
              danger: true,
              onSelect: onDelete,
            },
          ]}
        />
      </div>
    </div>
  );
}
