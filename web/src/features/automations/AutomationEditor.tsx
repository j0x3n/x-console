import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router";
import {
  ArrowDown,
  ArrowUp,
  Play,
  Plus,
  ShieldAlert,
  Trash2,
  X,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  isNotLive,
  useAutomation,
  useCatalog,
  useDeleteAutomation,
  useRunAutomation,
  useSaveAutomation,
  type AutomationInput,
  type Catalog,
  type Condition,
} from "./api";
import RunsList from "./components/RunsList";
import StepInput from "./components/StepInput";
import TriggerForm from "./components/TriggerForm";
import {
  cleanInput,
  CONDITION_OPS,
  emptyAutomation,
  formatCooldown,
  hasDangerous,
  schemaFields,
  validateAutomation,
} from "./logic";

export default function AutomationEditor() {
  const { id: idParam } = useParams();
  const id = idParam && idParam !== "new" ? Number(idParam) : null;
  const rule = useAutomation(id);
  const catalog = useCatalog();

  if ((id != null && rule.isPending) || catalog.isPending)
    return (
      <div className="xc-page">
        <Loading />
      </div>
    );
  if (catalog.isError || rule.isError) {
    const error = catalog.error ?? rule.error;
    return (
      <div className="xc-page">
        {isNotLive(error) ? (
          <p className="auto-muted">自动化还没上线。</p>
        ) : (
          <ErrorState
            error={error}
            onRetry={() =>
              catalog.isError ? catalog.refetch() : rule.refetch()
            }
          />
        )}
      </div>
    );
  }
  return (
    <Editor
      key={id ?? "new"}
      id={id}
      initial={id != null && rule.data ? rule.data : emptyAutomation()}
      authorized={rule.data?.authorized ?? false}
      catalog={catalog.data}
    />
  );
}

function Editor({
  id,
  initial,
  authorized,
  catalog,
}: {
  id: number | null;
  initial: AutomationInput;
  authorized: boolean;
  catalog: Catalog;
}) {
  const t = useT();
  const navigate = useNavigate();
  const save = useSaveAutomation();
  const remove = useDeleteAutomation();
  const run = useRunAutomation();
  const [form, setForm] = useState<AutomationInput>(() => strip(initial));
  const [tab, setTab] = useState<"rule" | "runs">("rule");
  const [errors, setErrors] = useState<string[]>([]);
  useEffect(() => setForm(strip(initial)), [initial]);

  const actions = useMemo(
    () => [...catalog.actions].sort((a, b) => a.name.localeCompare(b.name)),
    [catalog.actions],
  );
  const byName = useMemo(
    () => new Map(actions.map((a) => [a.name, a])),
    [actions],
  );
  const titles = useMemo(
    () => new Map(actions.map((a) => [a.name, a.title])),
    [actions],
  );
  const dangerous = hasDangerous(form.actions, actions);

  const set = <K extends keyof AutomationInput>(k: K, v: AutomationInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const body: AutomationInput = {
      ...form,
      name: form.name.trim(),
      actions: form.actions.map((s) => ({
        ...s,
        input: cleanInput(s.input, schemaFields(byName.get(s.action)?.input)),
      })),
    };
    const problems = validateAutomation(body);
    setErrors(problems);
    if (problems.length) return;
    save.mutate(
      { id, body },
      {
        onSuccess: (saved) => {
          toast(t("Saved"));
          if (id == null)
            navigate(`/automations/${saved.id}`, { replace: true });
        },
        onError: (err) => setErrors([errorMessage(err)]),
      },
    );
  };

  const setCondition = (i: number, patch: Partial<Condition>) =>
    set(
      "conditions",
      form.conditions.map((c, j) => (j === i ? { ...c, ...patch } : c)),
    );
  const moveStep = (i: number, d: -1 | 1) => {
    const next = [...form.actions];
    [next[i], next[i + d]] = [next[i + d], next[i]];
    set("actions", next);
  };

  return (
    <div className="xc-page auto-editor">
      <PageHeading
        title={id == null ? t("New rule") : form.name || t("Untitled rule")}
        aside={
          id != null && (
            <>
              <button
                type="button"
                className="xc-btn"
                disabled={run.isPending}
                onClick={() =>
                  run.mutate(id, { onSuccess: () => setTab("runs") })
                }
              >
                <Play size={14} /> {t("Run now")}
              </button>
              <button
                type="button"
                className="xc-btn danger"
                onClick={() => {
                  if (!confirm(`删除规则“${form.name}”？`)) return;
                  remove.mutate(id, {
                    onSuccess: () => navigate("/automations"),
                  });
                }}
              >
                <Trash2 size={14} />
              </button>
            </>
          )
        }
      />
      {id != null && (
        <nav className="xc-tabs">
          <button
            className={tab === "rule" ? "active" : ""}
            onClick={() => setTab("rule")}
          >
            {t("Rule")}
          </button>
          <button
            className={tab === "runs" ? "active" : ""}
            onClick={() => setTab("runs")}
          >
            {t("Run history")}
          </button>
        </nav>
      )}

      {tab === "runs" && id != null ? (
        <RunsList id={id} titles={titles} />
      ) : (
        <form className="auto-form" onSubmit={submit}>
          <section className="xc-card">
            <div className="auto-row">
              <label className="xc-field auto-grow">
                <span>{t("Name")}</span>
                <input
                  className="xc-input"
                  value={form.name}
                  onChange={(e) => set("name", e.target.value)}
                  placeholder="CPU 太高时提醒我"
                  required
                />
              </label>
              <label className="xc-field auto-narrow">
                <span>{t("Cooldown (seconds)")}</span>
                <input
                  className="xc-input"
                  type="number"
                  min={0}
                  value={form.cooldownSeconds}
                  onChange={(e) =>
                    set(
                      "cooldownSeconds",
                      Math.max(0, Number(e.target.value) || 0),
                    )
                  }
                />
                <small>
                  {formatCooldown(form.cooldownSeconds)}内不重复触发
                </small>
              </label>
            </div>
            <label className="xc-check">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => set("enabled", e.target.checked)}
              />
              <span>{t("Rule is on")}</span>
            </label>
          </section>

          <section className="xc-card">
            <div className="xc-card-head">
              <h2>1. {t("Rule trigger")}</h2>
            </div>
            <TriggerForm
              value={form.trigger}
              topics={catalog.topics}
              onChange={(v) => set("trigger", v)}
            />
          </section>

          <section className="xc-card">
            <div className="xc-card-head">
              <h2>2. {t("Only if")}</h2>
              <button
                type="button"
                className="xc-btn small"
                onClick={() =>
                  set("conditions", [
                    ...form.conditions,
                    { field: "", op: "==", value: "" },
                  ])
                }
              >
                <Plus size={13} /> {t("Add condition")}
              </button>
            </div>
            {form.conditions.length === 0 ? (
              <p className="auto-muted">没有条件，触发就执行。</p>
            ) : (
              <div className="auto-conditions">
                {form.conditions.map((c, i) => (
                  <div key={i} className="auto-condition">
                    <input
                      className="xc-input xc-mono"
                      aria-label={`${t("Field")} ${i + 1}`}
                      value={c.field}
                      placeholder="data.cpu"
                      onChange={(e) =>
                        setCondition(i, { field: e.target.value })
                      }
                    />
                    <select
                      className="xc-select"
                      aria-label={`${t("Compare")} ${i + 1}`}
                      value={c.op}
                      onChange={(e) =>
                        setCondition(i, {
                          op: e.target.value as Condition["op"],
                        })
                      }
                    >
                      {CONDITION_OPS.map((o) => (
                        <option key={o} value={o}>
                          {o === "contains" ? t("contains") : o}
                        </option>
                      ))}
                    </select>
                    <input
                      className="xc-input"
                      aria-label={`${t("Value")} ${i + 1}`}
                      value={c.value}
                      onChange={(e) =>
                        setCondition(i, { value: e.target.value })
                      }
                    />
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-label={`${t("Remove")} ${i + 1}`}
                      onClick={() =>
                        set(
                          "conditions",
                          form.conditions.filter((_, j) => j !== i),
                        )
                      }
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))}
                <small className="auto-muted">
                  字段写触发数据里的路径。全部满足才执行。
                </small>
              </div>
            )}
          </section>

          <section className="xc-card">
            <div className="xc-card-head">
              <h2>3. {t("Then do")}</h2>
            </div>
            {form.actions.map((step, i) => {
              const def = byName.get(step.action);
              return (
                <div key={i} className="auto-step">
                  <div className="auto-step-head">
                    <span className="auto-step-no">{i + 1}</span>
                    <select
                      className="xc-select"
                      aria-label={`${t("Rule action")} ${i + 1}`}
                      value={step.action}
                      onChange={(e) =>
                        set(
                          "actions",
                          form.actions.map((s, j) =>
                            j === i ? { action: e.target.value, input: {} } : s,
                          ),
                        )
                      }
                    >
                      <option value="">{t("Pick an action")}</option>
                      {actions.map((a) => (
                        <option key={a.name} value={a.name}>
                          {a.title}（{a.name}）
                        </option>
                      ))}
                    </select>
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-label={`${t("Move up")} ${i + 1}`}
                      disabled={i === 0}
                      onClick={() => moveStep(i, -1)}
                    >
                      <ArrowUp size={14} />
                    </button>
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-label={`${t("Move down")} ${i + 1}`}
                      disabled={i === form.actions.length - 1}
                      onClick={() => moveStep(i, 1)}
                    >
                      <ArrowDown size={14} />
                    </button>
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-label={`${t("Remove")} ${i + 1}`}
                      onClick={() =>
                        set(
                          "actions",
                          form.actions.filter((_, j) => j !== i),
                        )
                      }
                    >
                      <X size={14} />
                    </button>
                  </div>
                  {def?.effect === "dangerous" && (
                    <p className="auto-warn">
                      <ShieldAlert size={13} /> 高危动作，保存时要再验证一次。
                    </p>
                  )}
                  {def?.description && (
                    <p className="auto-muted auto-desc">{def.description}</p>
                  )}
                  {def && (
                    <StepInput
                      schema={def.input}
                      value={step.input}
                      onChange={(input) =>
                        set(
                          "actions",
                          form.actions.map((s, j) =>
                            j === i ? { ...s, input } : s,
                          ),
                        )
                      }
                    />
                  )}
                </div>
              );
            })}
            <button
              type="button"
              className="xc-btn small"
              onClick={() =>
                set("actions", [...form.actions, { action: "", input: {} }])
              }
            >
              <Plus size={13} /> {t("Add action")}
            </button>
            <small className="auto-muted auto-hint">
              {
                "输入里可以写 {{trigger.data.xxx}} 引用触发数据，{{steps.0.result}} 引用前面一步的结果。"
              }
            </small>
          </section>

          {errors.length > 0 && (
            <ul className="auto-errors" role="alert">
              {errors.map((e) => (
                <li key={e}>{e}</li>
              ))}
            </ul>
          )}
          <div className="auto-actions">
            {dangerous && id != null && !authorized && (
              <span className="auto-warn">
                <ShieldAlert size={13} />{" "}
                还没授权，高危动作不会执行。保存一次来授权。
              </span>
            )}
            <button className="xc-btn primary" disabled={save.isPending}>
              {t("Save")}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

/** 编辑时只保留可以提交的字段。 */
function strip(a: AutomationInput): AutomationInput {
  return {
    name: a.name,
    enabled: a.enabled,
    trigger: a.trigger,
    conditions: a.conditions,
    actions: a.actions,
    cooldownSeconds: a.cooldownSeconds,
  };
}
