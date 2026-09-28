import { useEffect, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  FileCode2,
  Pencil,
  Play,
  Plus,
  Trash2,
} from "lucide-react";
import { withElevation } from "../../../auth/elevation";
import Dialog from "../../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useDeleteScript,
  useRunScript,
  useScriptRuns,
  useScripts,
  type Script,
  type ScriptRun,
} from "../api";
import { runTone } from "../lib";
import { shortDateTime, showError, useIdParam, useParam } from "./common";
import HostPicker from "./HostPicker";
import ScriptDialog from "./ScriptDialog";

/** 脚本库：列表、新建、执行、运行记录。 */
export default function ScriptsTab() {
  const t = useT();
  const scripts = useScripts();
  const [creating, setCreating] = useParam("new");
  const [openId, setOpenId] = useIdParam("script");
  const [running, setRunning] = useState<Script | null>(null);
  const open = scripts.data?.find((s) => s.id === openId) ?? null;

  return (
    <>
      {scripts.isPending ? (
        <Loading />
      ) : scripts.isError ? (
        <ErrorState error={scripts.error} onRetry={() => scripts.refetch()} />
      ) : scripts.data.length === 0 ? (
        <EmptyState title={t("No scripts yet")} icon={<FileCode2 size={28} />}>
          <span>
            {t("Save a script once. Run it on many machines at the same time.")}
          </span>
          <button
            className="xc-btn primary small"
            onClick={() => setCreating("1")}
          >
            <Plus size={14} /> {t("New script")}
          </button>
        </EmptyState>
      ) : (
        <div className="xc-card monitoring-list">
          {scripts.data.map((s) => (
            <div key={s.id} className="monitoring-row monitoring-row-static">
              <button
                className="monitoring-row-main monitoring-link"
                onClick={() => setOpenId(s.id)}
              >
                <strong>
                  {s.name} <span className="xc-badge">{s.shell}</span>
                </strong>
                <small>{s.description || firstLine(s.body)}</small>
              </button>
              <span className="monitoring-row-side">
                <button className="xc-btn small" onClick={() => setRunning(s)}>
                  <Play size={13} /> {t("Run")}
                </button>
              </span>
            </div>
          ))}
        </div>
      )}
      <ScriptDialog
        open={creating === "1"}
        onClose={() => setCreating(null)}
        onSaved={(s) => setOpenId(s.id)}
      />
      <ScriptDetail
        script={open}
        onClose={() => setOpenId(null)}
        onRun={setRunning}
      />
      <RunDialog
        script={running}
        onClose={() => setRunning(null)}
        onStarted={(s) => {
          setRunning(null);
          setOpenId(s.id);
        }}
      />
    </>
  );
}

function firstLine(body: string) {
  return body.split("\n").find((l) => l.trim()) ?? "";
}

function RunDialog({
  script,
  onClose,
  onStarted,
}: {
  script: Script | null;
  onClose: () => void;
  onStarted: (s: Script) => void;
}) {
  const t = useT();
  const run = useRunScript();
  const [hosts, setHosts] = useState<string[]>([]);
  useEffect(() => {
    if (script) setHosts(script.defaultHostIds);
  }, [script]);
  if (!script) return null;
  const start = async () => {
    try {
      await withElevation(() =>
        run.mutateAsync({ id: script.id, hostIds: hosts }),
      );
      toast(t("Run started"));
      onStarted(script);
    } catch (err) {
      showError(err);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={`${t("Run")} · ${script.name}`}
      description={t(
        "The machines run it at the same time. You need to verify first.",
      )}
    >
      <div className="xc-field">
        <span>{t("Run on these machines")}</span>
        <HostPicker value={hosts} onChange={setHosts} />
      </div>
      <pre className="monitoring-code-view">{script.body}</pre>
      <div className="xc-dialog-actions">
        <button className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          className="xc-btn primary"
          disabled={hosts.length === 0 || run.isPending}
          onClick={start}
        >
          <Play size={14} />{" "}
          {t("Run on {n} machines").replace("{n}", String(hosts.length))}
        </button>
      </div>
    </Dialog>
  );
}

function ScriptDetail({
  script,
  onClose,
  onRun,
}: {
  script: Script | null;
  onClose: () => void;
  onRun: (s: Script) => void;
}) {
  const t = useT();
  const runs = useScriptRuns(script?.id ?? null);
  const remove = useDeleteScript();
  const [editing, setEditing] = useState(false);
  if (!script) return null;
  return (
    <>
      <Dialog
        open={!editing}
        onClose={onClose}
        title={script.name}
        description={script.description || undefined}
        wide
      >
        <pre className="monitoring-code-view">{script.body}</pre>
        <div className="monitoring-detail-bar">
          <strong>{t("Run history")}</strong>
          <span className="xc-muted">
            {script.shell} · {t("timeout")} {script.timeoutSeconds}s
          </span>
        </div>
        {runs.isPending ? (
          <Loading />
        ) : runs.isError ? (
          <ErrorState error={runs.error} onRetry={() => runs.refetch()} />
        ) : runs.data.items.length === 0 ? (
          <p className="xc-muted monitoring-small">{t("Not run yet")}</p>
        ) : (
          <div className="monitoring-runs">
            {runs.data.items.map((r) => (
              <RunRow key={r.id} run={r} />
            ))}
          </div>
        )}
        <div className="xc-dialog-actions monitoring-actions">
          <button
            className="xc-btn ghost danger"
            disabled={remove.isPending}
            onClick={() =>
              confirm(`${t("Delete")} “${script.name}”?`) &&
              remove.mutate(script.id, {
                onSuccess: () => {
                  toast(t("Deleted"));
                  onClose();
                },
                onError: showError,
              })
            }
          >
            <Trash2 size={14} /> {t("Delete")}
          </button>
          <span className="xc-spacer" />
          <button className="xc-btn" onClick={() => setEditing(true)}>
            <Pencil size={14} /> {t("Edit")}
          </button>
          <button className="xc-btn primary" onClick={() => onRun(script)}>
            <Play size={14} /> {t("Run")}
          </button>
        </div>
      </Dialog>
      <ScriptDialog
        open={editing}
        onClose={() => setEditing(false)}
        script={script}
      />
    </>
  );
}

const statusLabel: Record<ScriptRun["status"], string> = {
  running: "Running",
  ok: "Succeeded",
  failed: "Failed",
};

function RunRow({ run }: { run: ScriptRun }) {
  const t = useT();
  const language = useLanguage();
  const [expanded, setExpanded] = useState(false);
  const seconds =
    run.finishedAt &&
    Math.max(
      0,
      Math.round(
        (new Date(run.finishedAt).getTime() -
          new Date(run.startedAt).getTime()) /
          1000,
      ),
    );
  return (
    <div className="monitoring-run">
      <button
        className="monitoring-run-head"
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <span className={`xc-badge ${runTone(run)}`}>
          {t(statusLabel[run.status])}
        </span>
        <strong>{run.hostName || run.hostId}</strong>
        <span className="xc-spacer" />
        <small className="xc-muted">
          {shortDateTime(run.startedAt, language)}
          {seconds !== undefined && seconds !== null && ` · ${seconds}s`}
          {run.exitCode !== undefined && ` · ${t("exit")} ${run.exitCode}`}
        </small>
      </button>
      {expanded && (
        <div className="monitoring-run-body">
          {run.error && <p className="xc-error-text">{run.error}</p>}
          {run.stdout && <pre className="monitoring-output">{run.stdout}</pre>}
          {run.stderr && (
            <pre className="monitoring-output stderr">{run.stderr}</pre>
          )}
          {!run.error && !run.stdout && !run.stderr && (
            <p className="xc-muted monitoring-small">
              {run.status === "running" ? t("Still running") : t("No output")}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
