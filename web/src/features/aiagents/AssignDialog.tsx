import { useState } from "react";
import { Link, useNavigate } from "react-router";
import Dialog from "../../components/ui/Dialog";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { useAgents as useMachines } from "../../api/core";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useRepos } from "../coding/api";
import { useAiAgents, useAssignAgent } from "./api";
import AgentAvatar from "./AgentAvatar";
import { kindLabel } from "./logic";
import "./i18n";
import "./aiagents.css";

/** 卡片页的“分配给 Agent”（B47）。 */
export default function AssignDialog({
  issueKey,
  onClose,
}: {
  issueKey: string;
  onClose: () => void;
}) {
  const t = useT();
  const navigate = useNavigate();
  const agents = useAiAgents();
  const repos = useRepos();
  const machines = useMachines();
  const assign = useAssignAgent();
  const usable = (agents.data ?? []).filter((a) => a.enabled && !a.overBudget);
  const [agentId, setAgentId] = useState<number | undefined>();
  const agent = usable.find((a) => a.id === agentId) ?? usable[0];
  const cli = agent && agent.kind !== "builtin";
  const allowed = (repos.data ?? []).filter((r) =>
    agent?.repoIds.includes(r.id),
  );
  const [repoId, setRepoId] = useState<number | undefined>();
  const repo = allowed.find((r) => r.id === repoId) ?? allowed[0];
  const [base, setBase] = useState("");
  const [runner, setRunner] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const coding = (machines.data ?? []).filter((m) =>
    m.capabilities.includes("coding"),
  );

  return (
    <Dialog open onClose={onClose} title={t("Assign to an agent")} wide>
      {usable.length === 0 ? (
        <p className="xc-muted">
          {t("No agent can take work now.")}{" "}
          <Link to="/coding?new=1">{t("New agent")}</Link>
        </p>
      ) : (
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            if (!agent) return;
            setError("");
            try {
              const r = await assign.mutateAsync({
                agentId: agent.id,
                issueKey,
                repoId: cli ? repo?.id : undefined,
                baseBranch: cli && base.trim() ? base.trim() : undefined,
                runnerAgentId: cli && runner ? runner : undefined,
                note: note.trim() || undefined,
              });
              toast(t("The agent started"));
              onClose();
              if (r.taskId) navigate(`/coding/${r.taskId}`);
            } catch (err) {
              setError(errorMessage(err));
            }
          }}
        >
          <div className="xc-field">
            <span>{t("AI agent")}</span>
            <div className="aiagent-assign-list">
              {usable.map((a) => (
                <label key={a.id} className="elevation-mode">
                  <input
                    type="radio"
                    name="agent"
                    checked={agent?.id === a.id}
                    onChange={() => {
                      setAgentId(a.id);
                      setRepoId(undefined);
                    }}
                  />
                  <AgentAvatar agent={a} size={26} />
                  <span>
                    <strong>{a.name}</strong>
                    <small>
                      {t(kindLabel(a.kind))}
                      {a.runningTasks > 0
                        ? ` · ${t("Working")} ${a.runningTasks}`
                        : ""}
                    </small>
                  </span>
                </label>
              ))}
            </div>
          </div>
          {cli && (
            <>
              <label className="xc-field">
                <span>{t("Repository")}</span>
                <select
                  className="xc-select"
                  value={repo?.id ?? ""}
                  onChange={(e) => setRepoId(Number(e.target.value))}
                >
                  {allowed.length === 0 && (
                    <option value="">
                      {t("This agent may not change any repository")}
                    </option>
                  )}
                  {allowed.map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.remoteRepo ?? r.name} · {r.agentName}
                    </option>
                  ))}
                </select>
              </label>
              <div className="aiagent-form-row">
                <label className="xc-field">
                  <span>{t("Base branch")}</span>
                  <input
                    className="xc-input xc-mono"
                    value={base}
                    placeholder={repo?.defaultBranch || "main"}
                    onChange={(e) => setBase(e.target.value)}
                  />
                </label>
                <label className="xc-field">
                  <span>{t("Machine")}</span>
                  <select
                    className="xc-select"
                    value={runner}
                    onChange={(e) => setRunner(e.target.value)}
                  >
                    <option value="">
                      {agent?.runnerName
                        ? `${t("The agent's default")} (${agent.runnerName})`
                        : t("The repository's machine")}
                    </option>
                    {repo?.connectionId != null &&
                      coding.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name}
                        </option>
                      ))}
                  </select>
                </label>
              </div>
            </>
          )}
          <div className="xc-field">
            <span>{t("Extra notes")}</span>
            <MarkdownEditor
              label={t("Extra notes")}
              value={note}
              onChange={setNote}
              minRows={3}
              placeholder={t(
                "Optional. The card's title, description, open checklist items and recent comments are sent anyway.",
              )}
            />
          </div>
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-dialog-actions">
            <button type="button" className="xc-btn" onClick={onClose}>
              {t("Cancel")}
            </button>
            <button
              type="submit"
              className="xc-btn primary"
              disabled={!agent || (cli && !repo) || assign.isPending}
            >
              {t("Assign")}
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
