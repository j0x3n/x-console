import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import { useRuns, type Run } from "../api";

const STATUS: Record<Run["status"], { label: string; tone: string }> = {
  running: { label: "Running", tone: "info" },
  ok: { label: "Succeeded", tone: "ok" },
  failed: { label: "Failed", tone: "danger" },
  skipped: { label: "Run skipped", tone: "" },
};

export function RunBadge({ run }: { run: Run }) {
  const t = useT();
  const s = STATUS[run.status];
  return <span className={`xc-badge ${s.tone}`}>{t(s.label)}</span>;
}

export default function RunsList({
  id,
  titles,
}: {
  id: number;
  titles: Map<string, string>;
}) {
  const t = useT();
  const runs = useRuns(id);
  if (runs.isPending) return <Loading />;
  if (runs.isError)
    return <ErrorState error={runs.error} onRetry={() => runs.refetch()} />;
  if (runs.data.length === 0) return <EmptyState title={t("No runs yet")} />;
  return (
    <div className="auto-runs">
      {runs.data.map((run) => (
        <RunRow key={run.id} run={run} titles={titles} />
      ))}
    </div>
  );
}

function RunRow({ run, titles }: { run: Run; titles: Map<string, string> }) {
  const t = useT();
  const language = useLanguage();
  const [open, setOpen] = useState(run.status === "failed");
  const took = run.finishedAt
    ? (new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime()) /
      1000
    : null;
  return (
    <div className="auto-run">
      <button
        type="button"
        className="auto-run-head"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <RunBadge run={run} />
        <time dateTime={run.startedAt}>
          {relativeTime(run.startedAt, language)}
        </time>
        <span className="auto-muted">
          {run.steps.length} {t("steps")}
          {took != null ? ` · ${took.toFixed(1)} s` : ""}
        </span>
      </button>
      {open && (
        <div className="auto-run-body">
          {run.triggerData != null && (
            <details>
              <summary>{t("Trigger data")}</summary>
              <pre>{JSON.stringify(run.triggerData, null, 2)}</pre>
            </details>
          )}
          <ol>
            {run.steps.map((s, i) => (
              <li key={i} className={s.error ? "failed" : ""}>
                <strong>{titles.get(s.action) ?? s.action}</strong>
                {s.error ? (
                  <p className="auto-danger">{s.error}</p>
                ) : (
                  s.result != null && (
                    <pre>
                      {typeof s.result === "string"
                        ? s.result
                        : JSON.stringify(s.result, null, 2)}
                    </pre>
                  )
                )}
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}
