import { useState } from "react";
import { Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import ImageLightbox, {
  type LightboxImage,
} from "../../components/ui/ImageLightbox";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatBytes, relativeTime } from "../../lib/time";
import {
  shotUrl,
  useCleanupShots,
  useDeleteRunShots,
  useWatcherRuns,
  type ShotsKeep,
  type WatcherDetail,
  type WatcherRun,
} from "./api";
import { groupRuns, runShots } from "./lib";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

export const RUN_KIND: Record<WatcherRun["kind"], string> = {
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

type View = { images: LightboxImage[]; start: number };

const freed = (r: { deleted: number; bytes: number }) =>
  r.deleted > 0
    ? `删了 ${r.deleted} 张截图，腾出 ${formatBytes(r.bytes)}`
    : "没有要删的截图";

/**
 * B234：检查记录。默认折叠，连续一样的合成一行，先显示 10 条，
 * 点“加载更多”再取。截图在页面里看大图。
 */
export default function RunHistory({ w }: { w: WatcherDetail }) {
  const t = useT();
  const runs = useWatcherRuns(w.id);
  const [view, setView] = useState<View | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const list = runs.data?.pages.flatMap((p) => p.items) ?? w.runs.slice(0, 10);
  const groups = groupRuns(list);
  const shots = w.shots;

  return (
    <section className="xc-card">
      <div className="xc-card-head watchers-history-head">
        <h2>{t("Check history")}</h2>
        {shots && shots.count > 0 && (
          <>
            <span className="xc-muted watchers-shots-size">
              截图 {shots.count} 张，共 {formatBytes(shots.bytes)}
            </span>
            <button
              className="xc-btn small"
              title={t("Clean up screenshots")}
              onClick={() => setCleaning(true)}
            >
              <Trash2 size={14} /> {t("Clean up screenshots")}
            </button>
          </>
        )}
      </div>
      {list.length === 0 ? (
        <p className="xc-muted watchers-small">还没检查过。</p>
      ) : (
        <div className="watchers-runs">
          {groups.map((g) => (
            <RunItem
              key={g.latest.id}
              w={w}
              r={g.latest}
              count={g.count}
              onView={setView}
            />
          ))}
        </div>
      )}
      {runs.hasNextPage && (
        <button
          className="xc-btn ghost small watchers-more"
          disabled={runs.isFetchingNextPage}
          onClick={() => runs.fetchNextPage()}
        >
          {runs.isFetchingNextPage ? t("Loading") : t("Load more")}
        </button>
      )}
      <CleanupDialog open={cleaning} w={w} onClose={() => setCleaning(false)} />
      {view && (
        <ImageLightbox
          images={view.images}
          start={view.start}
          onClose={() => setView(null)}
        />
      )}
    </section>
  );
}

function RunItem({
  w,
  r,
  count,
  onView,
}: {
  w: WatcherDetail;
  r: WatcherRun;
  count: number;
  onView: (v: View) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const [open, setOpen] = useState(r.outcome === "running");
  const removeShots = useDeleteRunShots();
  const o = OUTCOME[r.outcome];
  const shots = runShots(r.steps);
  const images = shots.map((s) => ({ src: shotUrl(s.id), alt: s.text }));

  const onDeleteShots = async () => {
    const ok = await confirmAction({
      title: `删除这次${RUN_KIND[r.kind]}的 ${shots.length} 张截图？`,
      description: "检查记录还在，只是看不到截图了。",
      confirmLabel: t("Delete"),
    });
    if (ok)
      removeShots.mutate(
        { id: w.id, runId: r.id },
        { onSuccess: (res) => toast(freed(res)), onError: fail },
      );
  };

  return (
    <details
      className="watchers-run"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        <span className="watchers-run-kind">{RUN_KIND[r.kind]}</span>
        <span className={`xc-badge ${o.tone}`}>{o.label}</span>
        <span className="watchers-run-detail" title={r.detail}>
          {r.detail}
        </span>
        {count > 1 && <span className="xc-badge">同样 {count} 次</span>}
        <span className="xc-muted">{relativeTime(r.startedAt, language)}</span>
      </summary>
      {open && (
        <>
          {count > 1 && (
            <p className="xc-muted watchers-small">
              连续 {count} 次都是这样，下面是最近一次。
            </p>
          )}
          <ol className="watchers-steps">
            {r.steps.map((s, i) => (
              <li key={i}>
                <span>{s.text}</span>
                {s.shotId != null && (
                  <button
                    type="button"
                    className="watchers-thumb"
                    title={t("View screenshot")}
                    aria-label={`${t("View screenshot")}：${s.text}`}
                    onClick={() =>
                      onView({
                        images,
                        start: shots.findIndex((x) => x.id === s.shotId),
                      })
                    }
                  >
                    <img src={shotUrl(s.shotId)} alt="" loading="lazy" />
                  </button>
                )}
              </li>
            ))}
          </ol>
          <div className="watchers-run-foot">
            {r.tokens > 0 && (
              <span className="xc-muted">
                用了 {r.tokens.toLocaleString()} 个 token
              </span>
            )}
            {shots.length > 0 && r.outcome !== "running" && (
              <button
                type="button"
                className="xc-btn ghost small danger"
                disabled={removeShots.isPending}
                onClick={onDeleteShots}
              >
                <Trash2 size={14} /> {t("Delete screenshots")}
              </button>
            )}
          </div>
        </>
      )}
    </details>
  );
}

/** 清理这个任务的截图：只留最近一次成功的，或者全部删掉。 */
function CleanupDialog({
  open,
  w,
  onClose,
}: {
  open: boolean;
  w: WatcherDetail;
  onClose: () => void;
}) {
  const t = useT();
  const cleanup = useCleanupShots();
  const shots = w.shots;
  const run = (keep: ShotsKeep) =>
    cleanup.mutate(
      { id: w.id, keep },
      {
        onSuccess: (res) => {
          toast(freed(res));
          onClose();
        },
        onError: fail,
      },
    );
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Clean up screenshots")}
      description={
        shots
          ? `现在有 ${shots.count} 张截图，共 ${formatBytes(shots.bytes)}。删掉后检查记录还在，只是看不到截图。`
          : undefined
      }
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn danger"
            disabled={cleanup.isPending}
            onClick={() => run("none")}
          >
            {t("Delete all")}
          </button>
          <button
            className="xc-btn primary"
            disabled={cleanup.isPending}
            onClick={() => run("latest")}
          >
            {t("Keep the latest success")}
          </button>
        </>
      }
    >
      <p className="watchers-small">
        “只留最近一次成功的”会留下最近一次成功检查的截图，还有变化记录里的截图。“全部删掉”连变化记录的截图也删。
      </p>
    </Dialog>
  );
}
