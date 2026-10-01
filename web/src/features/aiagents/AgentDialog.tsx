import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { useAgents as useMachines } from "../../api/core";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useRepos } from "../coding/api";
import { useAiModels, useAiProviders } from "../assistant/api";
import ModelPicker from "../assistant/settings/ModelPicker";
import {
  useAgentMutations,
  type AiAgent,
  type AiAgentInput,
  type AiAgentKind,
} from "./api";
import { AVATARS, COLORS, KINDS } from "./logic";

const ACCESS = [
  { value: "read", label: "Read only" },
  { value: "write", label: "Read and write" },
  { value: "write_delete", label: "Read, write and delete" },
] as const;

/** 新建或编辑 Agent（B47）。 */
export default function AgentDialog({
  agent,
  onClose,
}: {
  agent?: AiAgent;
  onClose: () => void;
}) {
  const t = useT();
  const ops = useAgentMutations();
  const machines = useMachines();
  const repos = useRepos();
  const [kind, setKind] = useState<AiAgentKind>(agent?.kind ?? "claude_code");
  const builtin = kind === "builtin";
  const providers = useAiProviders(builtin);
  const models = useAiModels(builtin);
  const [name, setName] = useState(agent?.name ?? "");
  const [avatar, setAvatar] = useState(agent?.avatar ?? AVATARS[0]);
  const [color, setColor] = useState(agent?.color ?? COLORS[0]);
  const [model, setModel] = useState(agent?.model ?? "");
  const [instructions, setInstructions] = useState(agent?.instructions ?? "");
  const [runner, setRunner] = useState(agent?.runnerAgentId ?? "");
  const [access, setAccess] = useState(agent?.access ?? "write");
  const [permission, setPermission] = useState(
    agent?.cliPermission ?? "workspace",
  );
  const [repoIds, setRepoIds] = useState<number[]>(agent?.repoIds ?? []);
  const [parallel, setParallel] = useState(agent?.maxParallel ?? 1);
  const [budget, setBudget] = useState(
    agent?.monthlyBudgetUsd != null ? String(agent.monthlyBudgetUsd) : "",
  );
  const [autoBuild, setAutoBuild] = useState(agent?.autoBuild ?? true);
  const [retries, setRetries] = useState(agent?.buildRetries ?? 2);
  const [error, setError] = useState("");

  const coding = (machines.data ?? []).filter((m) =>
    m.capabilities.includes("coding"),
  );
  const [providerId, modelId] = model.includes(":")
    ? [Number(model.split(":")[0]), model.slice(model.indexOf(":") + 1)]
    : [0, ""];

  const submit = async () => {
    setError("");
    const body: AiAgentInput = {
      name: name.trim(),
      avatar,
      color,
      model: model.trim(),
      instructions,
      maxParallel: parallel,
      monthlyBudgetUsd: budget.trim() === "" ? null : Number(budget),
    };
    if (!agent) body.kind = kind;
    if (builtin) {
      body.access = access;
    } else {
      body.runnerAgentId = runner || null;
      body.cliPermission = permission;
      body.repoIds = repoIds;
      body.autoBuild = autoBuild;
      body.buildRetries = retries;
    }
    try {
      if (agent) await ops.update.mutateAsync({ id: agent.id, body });
      else await ops.create.mutateAsync(body);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open
      onClose={onClose}
      title={agent ? t("Edit agent") : t("New agent")}
      wide
    >
      <form
        className="aiagent-form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <div className="aiagent-form-row">
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              maxLength={40}
              required
              autoFocus
              placeholder={t("For example: Backend developer")}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <div className="xc-field">
            <span>{t("Avatar")}</span>
            <div className="aiagent-picks">
              {AVATARS.map((a) => (
                <button
                  type="button"
                  key={a}
                  className={`aiagent-pick${avatar === a ? " on" : ""}`}
                  aria-pressed={avatar === a}
                  onClick={() => setAvatar(a)}
                >
                  {a}
                </button>
              ))}
            </div>
            <div className="aiagent-picks">
              {COLORS.map((c) => (
                <button
                  type="button"
                  key={c}
                  className={`aiagent-swatch${color === c ? " on" : ""}`}
                  style={{ background: c }}
                  aria-label={c}
                  aria-pressed={color === c}
                  onClick={() => setColor(c)}
                />
              ))}
            </div>
          </div>
        </div>
        <div className="xc-field">
          <span>{t("Type")}</span>
          <div className="aiagent-kinds">
            {KINDS.map((k) => (
              <label key={k.value} className="elevation-mode">
                <input
                  type="radio"
                  name="kind"
                  checked={kind === k.value}
                  disabled={!!agent && agent.kind !== k.value}
                  onChange={() => {
                    setKind(k.value);
                    setModel("");
                  }}
                />
                <span>
                  <strong>{t(k.label)}</strong>
                  <small>{t(k.hint)}</small>
                </span>
              </label>
            ))}
          </div>
          {agent && (
            <small>
              {t("The type cannot change after the agent is created.")}
            </small>
          )}
        </div>
        {builtin ? (
          <div className="xc-field">
            <span>{t("Model")}</span>
            <ModelPicker
              label={t("Model")}
              value={providerId ? { providerId, model: modelId } : undefined}
              onChange={(v) => setModel(v ? `${v.providerId}:${v.model}` : "")}
              models={models.data ?? []}
              providers={providers.data ?? []}
              disabled={(m) =>
                m.toolCall === false ? t("No tool calls") : null
              }
            />
            <small>
              {t(
                "Pick one of your AI providers' models. It must support tool calls.",
              )}
            </small>
          </div>
        ) : (
          <label className="xc-field">
            <span>{t("Model")}</span>
            <input
              className="xc-input"
              value={model}
              placeholder={kind === "codex" ? "gpt-5-codex" : "opus"}
              onChange={(e) => setModel(e.target.value)}
            />
            <small>
              {t(
                "Passed to the command line as --model. Leave empty for its default.",
              )}
            </small>
          </label>
        )}
        <div className="xc-field">
          <span>{t("Instructions")}</span>
          <MarkdownEditor
            label={t("Instructions")}
            value={instructions}
            onChange={setInstructions}
            minRows={4}
            placeholder={t(
              "Put at the top of every task. For example: You are the backend developer. Only change the backend folder and run go test before you finish.",
            )}
          />
        </div>
        {builtin ? (
          <div className="xc-field">
            <span>{t("Access")}</span>
            <div className="aiagent-kinds">
              {ACCESS.map((a) => (
                <label key={a.value} className="elevation-mode">
                  <input
                    type="radio"
                    name="access"
                    checked={access === a.value}
                    onChange={() => setAccess(a.value)}
                  />
                  <span>
                    <strong>{t(a.label)}</strong>
                  </span>
                </label>
              ))}
            </div>
            <small>
              {t(
                "The same rules as API tokens. Running commands, power and settings are never available.",
              )}
            </small>
          </div>
        ) : (
          <>
            <label className="xc-field">
              <span>{t("Default machine")}</span>
              <select
                className="xc-select"
                value={runner}
                onChange={(e) => setRunner(e.target.value)}
              >
                <option value="">{t("The repository's machine")}</option>
                {coding.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                    {m.online ? "" : ` (${t("offline")})`}
                  </option>
                ))}
              </select>
              <small>
                {t("Only machines that can run Claude Code or Codex.")}
              </small>
            </label>
            <div className="xc-field">
              <span>{t("Repositories it may change")}</span>
              {(repos.data ?? []).length === 0 ? (
                <small>
                  {t("No repositories yet. Add one on the Repositories tab.")}
                </small>
              ) : (
                <div className="aiagent-repos">
                  {(repos.data ?? []).map((r) => (
                    <label key={r.id} className="xc-check">
                      <input
                        type="checkbox"
                        checked={repoIds.includes(r.id)}
                        onChange={(e) =>
                          setRepoIds(
                            e.target.checked
                              ? [...repoIds, r.id]
                              : repoIds.filter((x) => x !== r.id),
                          )
                        }
                      />
                      <span>
                        {r.remoteRepo ?? r.name}
                        <small className="xc-muted"> · {r.agentName}</small>
                      </span>
                    </label>
                  ))}
                </div>
              )}
            </div>
            <div className="xc-field">
              <span>{t("Command line permission")}</span>
              <div className="aiagent-kinds">
                <label className="elevation-mode">
                  <input
                    type="radio"
                    name="permission"
                    checked={permission === "workspace"}
                    onChange={() => setPermission("workspace")}
                  />
                  <span>
                    <strong>{t("Work folder only")}</strong>
                    <small>
                      {t("Can only change files in the task's work folder.")}
                    </small>
                  </span>
                </label>
                <label className="elevation-mode">
                  <input
                    type="radio"
                    name="permission"
                    checked={permission === "full"}
                    onChange={() => setPermission("full")}
                  />
                  <span>
                    <strong>{t("Full")}</strong>
                    <small>
                      {t(
                        "No sandbox and no questions. It can run any command on the machine.",
                      )}
                    </small>
                  </span>
                </label>
              </div>
              {permission === "full" && (
                <p className="aiagent-warning" role="note">
                  <AlertTriangle size={14} />
                  {t("Use only on a machine you can afford to break.")}
                </p>
              )}
            </div>
            <div className="aiagent-form-row">
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={autoBuild}
                  onChange={(e) => setAutoBuild(e.target.checked)}
                />
                {t("Build after each change")}
              </label>
              <label className="xc-field aiagent-small-field">
                <span>{t("Retries after a failed build")}</span>
                <input
                  className="xc-input"
                  type="number"
                  min={0}
                  max={5}
                  value={retries}
                  disabled={!autoBuild}
                  onChange={(e) => setRetries(Number(e.target.value))}
                />
              </label>
            </div>
          </>
        )}
        <div className="aiagent-form-row">
          <label className="xc-field aiagent-small-field">
            <span>{t("Tasks at the same time")}</span>
            <input
              className="xc-input"
              type="number"
              min={1}
              max={10}
              value={parallel}
              onChange={(e) => setParallel(Number(e.target.value))}
            />
          </label>
          <label className="xc-field aiagent-small-field">
            <span>{t("Monthly budget (USD)")}</span>
            <input
              className="xc-input"
              type="number"
              min={0}
              step="0.01"
              value={budget}
              placeholder={t("No limit")}
              onChange={(e) => setBudget(e.target.value)}
            />
          </label>
        </div>
        <small className="xc-muted">
          {t(
            "When the month's cost reaches the budget, the agent takes no new tasks. Running ones finish.",
          )}
        </small>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={
              !name.trim() ||
              (builtin && !model) ||
              ops.create.isPending ||
              ops.update.isPending
            }
          >
            {agent ? t("Save") : t("Create agent")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
