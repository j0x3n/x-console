import { Download, Hammer } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import { toast } from "../../../hooks/useToast";
import { useBuildTask, useRepos, type Task } from "../api";

const BADGE: Record<string, { tone: string; label: string }> = {
  running: { tone: "info", label: "Building" },
  passed: { tone: "ok", label: "Build passed" },
  failed: { tone: "danger", label: "Build failed" },
};

/** B47：任务的构建状态、手动构建和产物下载。 */
export default function BuildCard({ task }: { task: Task }) {
  const t = useT();
  const repos = useRepos();
  const build = useBuildTask();
  const repo = repos.data?.find((r) => r.id === task.repoId);
  const steps =
    (repo?.buildConfig?.linux.length ?? 0) +
    (repo?.buildConfig?.windows.length ?? 0);
  const status = task.buildStatus ?? "";
  if (!steps && !status) return null;
  const badge = BADGE[status];
  const canBuild =
    task.status === "review" && status !== "running" && steps > 0;
  const artifacts = task.artifacts ?? [];
  return (
    <section className="xc-card coding-build">
      <div className="xc-card-head">
        <h2>{t("Build")}</h2>
        {badge && (
          <span className={`xc-badge ${badge.tone}`}>{t(badge.label)}</span>
        )}
      </div>
      {task.buildAttempts ? (
        <small className="xc-muted">
          {t("Builds")}: {task.buildAttempts}
        </small>
      ) : null}
      {task.buildError && (
        <p className="coding-build-error">{task.buildError}</p>
      )}
      {artifacts.length > 0 && (
        <ul className="coding-artifacts">
          {artifacts.map((a, i) => (
            <li key={a.name}>
              <a
                className="xc-btn ghost small"
                href={`/api/v1/coding/tasks/${task.id}/artifacts/${i}`}
                download
              >
                <Download size={14} />
                <span className="xc-mono">{a.name}</span>
              </a>
              <small className="xc-muted">{formatBytes(a.size)}</small>
            </li>
          ))}
        </ul>
      )}
      {canBuild && (
        <button
          className="xc-btn small"
          disabled={build.isPending}
          onClick={() =>
            build.mutate(task.id, {
              onError: (e) =>
                toast({ message: errorMessage(e), tone: "error" }),
            })
          }
        >
          <Hammer size={14} /> {t("Build now")}
        </button>
      )}
      {!steps && (
        <small className="xc-muted">
          {t("The repository has no build steps any more.")}
        </small>
      )}
    </section>
  );
}
