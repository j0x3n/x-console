import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useCreateTask,
  useExecutors,
  useRepos,
  type ExecutorName,
} from "../api";

const LAST_KEY = "xc.coding.last";

interface Last {
  repoId?: number;
  executor?: ExecutorName;
}

function readLast(): Last {
  try {
    return JSON.parse(localStorage.getItem(LAST_KEY) ?? "{}") as Last;
  } catch {
    return {};
  }
}

function saveLast(last: Last) {
  try {
    localStorage.setItem(LAST_KEY, JSON.stringify(last));
  } catch {
    /* 存不了就算了 */
  }
}

interface Props {
  open: boolean;
  onClose: () => void;
  /** 从 Issue 页进来时带上 Issue 编号。 */
  issueKey?: string;
}

export default function NewTaskDialog({ open, onClose, issueKey }: Props) {
  const t = useT();
  const navigate = useNavigate();
  const repos = useRepos();
  const create = useCreateTask();
  const [repoId, setRepoId] = useState<number | undefined>();
  const [executor, setExecutor] = useState<ExecutorName>("claude");
  const [baseBranch, setBaseBranch] = useState("");
  const [prompt, setPrompt] = useState("");

  useEffect(() => {
    if (!open) return;
    const last = readLast();
    setExecutor(last.executor ?? "claude");
    setPrompt("");
    setBaseBranch("");
    setRepoId(last.repoId);
  }, [open]);

  const list = repos.data ?? [];
  const repo = list.find((r) => r.id === repoId) ?? list[0];
  const executors = useExecutors(repo?.agentId, open && !!repo?.agentOnline);
  const chosen = executors.data?.find((e) => e.name === executor);

  const submit = async (event: { preventDefault: () => void }) => {
    event.preventDefault();
    if (!repo) return;
    try {
      const task = await create.mutateAsync({
        repoId: repo.id,
        executor,
        prompt: prompt.trim() || undefined,
        baseBranch: baseBranch.trim() || undefined,
        issueKey,
      });
      saveLast({ repoId: repo.id, executor });
      toast(task.status === "queued" ? t("Task queued") : t("Task started"));
      onClose();
      navigate(`/coding/${task.id}`);
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };

  const canSubmit =
    !!repo && (prompt.trim() !== "" || !!issueKey) && !create.isPending;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("New coding task")}
      description={
        issueKey
          ? `${issueKey} · ${t("Its title and description are added to the prompt.")}`
          : undefined
      }
      wide
    >
      {repos.isSuccess && list.length === 0 ? (
        <div className="xc-stack">
          <p className="xc-muted">{t("Register a repository first.")}</p>
          <div className="xc-dialog-actions">
            <button className="xc-btn" onClick={onClose}>
              {t("Cancel")}
            </button>
            <Link
              className="xc-btn primary"
              to="/coding/repos"
              onClick={onClose}
            >
              {t("Manage repositories")}
            </Link>
          </div>
        </div>
      ) : (
        <form onSubmit={submit}>
          <div className="coding-form-row">
            <label className="xc-field">
              <span>{t("Repository")}</span>
              <select
                className="xc-select"
                value={repo?.id ?? ""}
                onChange={(e) => setRepoId(Number(e.target.value))}
              >
                {list.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name} · {r.agentName || r.agentId}
                    {r.agentOnline ? "" : ` (${t("offline")})`}
                  </option>
                ))}
              </select>
              {repo && !repo.agentOnline && (
                <small>
                  {t("The machine is offline. The task waits in the queue.")}
                </small>
              )}
            </label>
            <label className="xc-field">
              <span>{t("Executor")}</span>
              <select
                className="xc-select"
                value={executor}
                onChange={(e) => setExecutor(e.target.value as ExecutorName)}
              >
                {(["claude", "codex"] as const).map((name) => {
                  const info = executors.data?.find((e) => e.name === name);
                  return (
                    <option key={name} value={name}>
                      {name === "claude" ? "Claude Code" : "Codex"}
                      {info && !info.available
                        ? ` (${t("not found")})`
                        : info?.version
                          ? ` · ${info.version}`
                          : ""}
                    </option>
                  );
                })}
              </select>
              {chosen && !chosen.available && (
                <small className="xc-error-text">
                  {t("This executor is not installed on the machine.")}
                </small>
              )}
            </label>
            <label className="xc-field">
              <span>{t("Base branch")}</span>
              <input
                className="xc-input"
                value={baseBranch}
                placeholder={repo?.defaultBranch || "HEAD"}
                onChange={(e) => setBaseBranch(e.target.value)}
              />
            </label>
          </div>
          <label className="xc-field">
            <span>{t("What should it do?")}</span>
            <textarea
              className="xc-textarea coding-prompt"
              rows={7}
              value={prompt}
              autoFocus
              placeholder={
                issueKey
                  ? t("Anything to add? Optional.")
                  : t("Describe the change you want.")
              }
              onChange={(e) => setPrompt(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && canSubmit)
                  submit(e);
              }}
            />
            <small>
              {t(
                "It runs in its own git worktree. Your checkout is not touched.",
              )}
            </small>
          </label>
          <div className="xc-dialog-actions">
            <button type="button" className="xc-btn" onClick={onClose}>
              {t("Cancel")}
            </button>
            <button
              type="submit"
              className="xc-btn primary"
              disabled={!canSubmit}
            >
              {t("Start task")}
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
