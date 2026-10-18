import { useEffect, useState } from "react";
import { Play, RefreshCw } from "lucide-react";
import { ApiError, errorMessage, isNotLive } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDispatchWorkflow,
  useWorkflows,
  type GitHubWorkflow,
  type GitHubWorkflowInput,
  type RepoRef,
} from "./api";

/** 每个输入参数的初始值：有默认值用默认值，勾选框默认不勾，选项默认第一个。 */
export function initialInputs(wf: GitHubWorkflow): Record<string, string> {
  const values: Record<string, string> = {};
  for (const input of wf.inputs) {
    if (input.default != null) values[input.name] = input.default;
    else if (input.type === "boolean") values[input.name] = "false";
    else if (input.type === "choice")
      values[input.name] = input.options[0] ?? "";
    else values[input.name] = "";
  }
  return values;
}

/** 用户取消二次验证时不算出错，不显示提示。 */
function canceled(error: unknown) {
  return error instanceof ApiError && error.code === "elevation_canceled";
}

/** B248：手动运行一个仓库的工作流。只列有 workflow_dispatch 的。 */
export default function RunWorkflowDialog({
  repo,
  defaultBranch,
  onClose,
}: {
  repo: RepoRef;
  defaultBranch: string;
  onClose: () => void;
}) {
  const t = useT();
  const { query, refresh } = useWorkflows(repo);
  const dispatch = useDispatchWorkflow(repo);
  const workflows = query.data?.items ?? [];
  const [workflowId, setWorkflowId] = useState(0);
  const [branch, setBranch] = useState(defaultBranch);
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const current = workflows.find((w) => w.id === workflowId);

  // 列表到了或刷新后，没选中的默认选第一个
  useEffect(() => {
    if (!workflows.some((w) => w.id === workflowId) && workflows[0]) {
      setWorkflowId(workflows[0].id);
      setInputs(initialInputs(workflows[0]));
    }
  }, [query.data]);

  const pick = (id: number) => {
    setWorkflowId(id);
    const wf = workflows.find((w) => w.id === id);
    setInputs(wf ? initialInputs(wf) : {});
    setError("");
  };

  const onSubmit = async () => {
    if (!current) return;
    setError("");
    try {
      await dispatch.mutateAsync({
        workflowId: current.id,
        ref: branch.trim(),
        inputs,
      });
      toast(t("Submitted. It shows up in CI runs in a moment."));
      onClose();
    } catch (err) {
      if (!canceled(err)) setError(errorMessage(err));
    }
  };

  let body;
  if (query.isPending) body = <Loading />;
  else if (query.isError)
    body = isNotLive(query.error) ? (
      <NotLive name={t("Run workflow")} icon={<Play size={28} />} />
    ) : (
      <p className="xc-error-text">{errorMessage(query.error)}</p>
    );
  else if (workflows.length === 0)
    body = (
      <p className="xc-muted">
        {t(
          "No workflow in this repository can be run by hand. Add workflow_dispatch under on: in the workflow file.",
        )}
      </p>
    );
  else
    body = (
      <>
        <label className="xc-field">
          <span>{t("Workflow")}</span>
          <select
            className="xc-select"
            value={workflowId}
            onChange={(e) => pick(Number(e.target.value))}
          >
            {workflows.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
          </select>
          {current && <small className="xc-mono">{current.path}</small>}
        </label>
        <label className="xc-field">
          <span>{t("Branch")}</span>
          <input
            className="xc-input xc-mono"
            value={branch}
            required
            onChange={(e) => setBranch(e.target.value)}
          />
        </label>
        {current?.inputs.map((input) => (
          <InputField
            key={`${current.id}:${input.name}`}
            input={input}
            value={inputs[input.name] ?? ""}
            onChange={(v) =>
              setInputs((prev) => ({ ...prev, [input.name]: v }))
            }
          />
        ))}
      </>
    );

  return (
    <Dialog open onClose={onClose} title={`${t("Run workflow")}：${repo.repo}`}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void onSubmit();
        }}
      >
        {body}
        {error && <p className="xc-error-text repos-run-error">{error}</p>}
        <div className="xc-dialog-actions">
          <button
            type="button"
            className="xc-btn ghost repos-run-refresh"
            disabled={refresh.isPending || query.isPending}
            title={t("Reload the workflow list")}
            onClick={() => refresh.mutate()}
          >
            <RefreshCw
              size={14}
              className={refresh.isPending ? "github-spin" : ""}
            />
            {t("Refresh")}
          </button>
          <span className="xc-spacer" />
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!current || !branch.trim() || dispatch.isPending}
          >
            <Play size={14} /> {t("Start run")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** 一个输入参数：勾选、选项、数字、文字。 */
function InputField({
  input,
  value,
  onChange,
}: {
  input: GitHubWorkflowInput;
  value: string;
  onChange: (value: string) => void;
}) {
  const hint = input.description && input.description !== input.name && (
    <small>{input.description}</small>
  );
  if (input.type === "boolean")
    return (
      <>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={value === "true"}
            onChange={(e) => onChange(e.target.checked ? "true" : "false")}
          />
          <span className="xc-mono">{input.name}</span>
        </label>
        {input.description && input.description !== input.name && (
          <small className="xc-check-hint">{input.description}</small>
        )}
      </>
    );
  const label = (
    <span>
      <span className="xc-mono">{input.name}</span>
      {input.required && <span className="xc-muted"> *</span>}
    </span>
  );
  if (input.type === "choice")
    return (
      <label className="xc-field">
        {label}
        <select
          className="xc-select"
          value={value}
          required={input.required}
          onChange={(e) => onChange(e.target.value)}
        >
          {!input.options.includes(value) && (
            <option value={value}>{value}</option>
          )}
          {input.options.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
        {hint}
      </label>
    );
  return (
    <label className="xc-field">
      {label}
      <input
        className="xc-input"
        type={input.type === "number" ? "number" : "text"}
        value={value}
        required={input.required}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint}
    </label>
  );
}
