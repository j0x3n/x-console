import { useEffect, useState, type FormEvent } from "react";
import { Plus, Play, Trash2 } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading } from "../../components/ui/States";
import { withElevation } from "../../auth/elevation";
import { errorMessage } from "../../api/client";
import { toast } from "../../hooks/useToast";
import {
  useAutomationCatalog,
  useAutomationRuns,
  useAutomations,
  useDeleteAutomation,
  useRunAutomation,
  useSaveAutomation,
  type ActionCatalogItem,
  type Automation,
  type AutomationInput,
  type Condition,
  type Step,
  type Trigger,
} from "./api";

const blank = (): AutomationInput => ({
  name: "",
  enabled: true,
  trigger: { type: "event", topic: "host.alert" },
  conditions: [],
  actions: [],
  cooldownSeconds: 60,
});

function ActionInput({
  step,
  catalog,
  change,
}: {
  step: Step;
  catalog: ActionCatalogItem[];
  change: (input: Record<string, unknown>) => void;
}) {
  const schema = catalog.find((item) => item.name === step.action)?.input as
    | { properties?: Record<string, { type?: string; description?: string }> }
    | undefined;
  const properties = schema?.properties;
  const simple =
    properties &&
    Object.keys(properties).length <= 8 &&
    Object.values(properties).every((value) =>
      ["string", "number", "integer", "boolean"].includes(value.type ?? ""),
    );
  const [json, setJson] = useState(JSON.stringify(step.input, null, 2));
  useEffect(() => setJson(JSON.stringify(step.input, null, 2)), [step.action]);
  if (!simple)
    return (
      <label className="xc-field">
        <span>输入 JSON</span>
        <textarea
          className="xc-textarea"
          rows={4}
          value={json}
          onChange={(event) => {
            setJson(event.target.value);
            try {
              const value = JSON.parse(event.target.value);
              if (value && typeof value === "object" && !Array.isArray(value))
                change(value);
            } catch {
              /* 输入未完成 */
            }
          }}
        />
      </label>
    );
  return (
    <div className="automation-fields">
      {Object.entries(properties).map(([key, property]) => (
        <label className="xc-field" key={key}>
          <span>
            {key}
            {property.description ? ` · ${property.description}` : ""}
          </span>
          {property.type === "boolean" ? (
            <input
              type="checkbox"
              checked={Boolean(step.input[key])}
              onChange={(event) =>
                change({ ...step.input, [key]: event.target.checked })
              }
            />
          ) : (
            <input
              className="xc-input"
              type={
                property.type === "number" || property.type === "integer"
                  ? "number"
                  : "text"
              }
              value={String(step.input[key] ?? "")}
              onChange={(event) =>
                change({
                  ...step.input,
                  [key]:
                    property.type === "number" || property.type === "integer"
                      ? Number(event.target.value)
                      : event.target.value,
                })
              }
            />
          )}
        </label>
      ))}
    </div>
  );
}

function RuleEditor({
  rule,
  catalog,
  close,
  saved,
}: {
  rule?: Automation;
  catalog: ActionCatalogItem[];
  close: () => void;
  saved: (item: Automation) => void;
}) {
  const [form, setForm] = useState<AutomationInput>(() =>
    rule
      ? {
          name: rule.name,
          enabled: rule.enabled,
          trigger: rule.trigger,
          conditions: rule.conditions,
          actions: rule.actions,
          cooldownSeconds: rule.cooldownSeconds,
        }
      : blank(),
  );
  const save = useSaveAutomation();
  const trigger = form.trigger;
  const setTrigger = (patch: Partial<Trigger>) =>
    setForm((value) => ({ ...value, trigger: { ...value.trigger, ...patch } }));
  const setCondition = (index: number, patch: Partial<Condition>) =>
    setForm((value) => ({
      ...value,
      conditions: value.conditions.map((item, i) =>
        i === index ? { ...item, ...patch } : item,
      ),
    }));
  const setStep = (index: number, patch: Partial<Step>) =>
    setForm((value) => ({
      ...value,
      actions: value.actions.map((item, i) =>
        i === index ? { ...item, ...patch } : item,
      ),
    }));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    try {
      const value = await withElevation(() =>
        save.mutateAsync({ id: rule?.id, body: form }),
      );
      toast("规则已保存");
      saved(value);
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <form className="xc-card automation-editor" onSubmit={submit}>
      <h2>{rule ? "编辑规则" : "新建规则"}</h2>
      <div className="automation-fields">
        <label className="xc-field">
          <span>名称</span>
          <input
            className="xc-input"
            value={form.name}
            onChange={(event) => setForm({ ...form, name: event.target.value })}
            required
          />
        </label>
        <label className="xc-field">
          <span>冷却秒数</span>
          <input
            className="xc-input"
            type="number"
            min="0"
            value={form.cooldownSeconds}
            onChange={(event) =>
              setForm({ ...form, cooldownSeconds: Number(event.target.value) })
            }
          />
        </label>
        <label className="xc-field">
          <span>启用</span>
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(event) =>
              setForm({ ...form, enabled: event.target.checked })
            }
          />
        </label>
      </div>
      <h3>触发器</h3>
      <div className="automation-fields">
        <label className="xc-field">
          <span>类型</span>
          <select
            className="xc-select"
            value={trigger.type}
            onChange={(event) =>
              setForm({
                ...form,
                trigger: { type: event.target.value as Trigger["type"] },
              })
            }
          >
            <option value="schedule">定时</option>
            <option value="event">事件</option>
            <option value="metric">指标</option>
            <option value="ha_state">HA 状态</option>
            <option value="webhook">Webhook</option>
          </select>
        </label>
        {trigger.type === "schedule" && (
          <label className="xc-field">
            <span>Cron 表达式</span>
            <input
              className="xc-input"
              value={trigger.cron ?? ""}
              onChange={(event) => setTrigger({ cron: event.target.value })}
              placeholder="0 8 * * *"
              required
            />
          </label>
        )}
        {(trigger.type === "event" || trigger.type === "metric") && (
          <>
            <label className="xc-field">
              <span>事件主题前缀</span>
              <input
                className="xc-input"
                value={
                  trigger.topic ??
                  (trigger.type === "metric" ? "host.metrics" : "")
                }
                onChange={(event) => setTrigger({ topic: event.target.value })}
                required
              />
            </label>
            <label className="xc-field">
              <span>字段路径（可选）</span>
              <input
                className="xc-input"
                value={trigger.field ?? ""}
                onChange={(event) => setTrigger({ field: event.target.value })}
                placeholder="data.cpu"
              />
            </label>
            <label className="xc-field">
              <span>比较</span>
              <select
                className="xc-select"
                value={trigger.op ?? "gt"}
                onChange={(event) => setTrigger({ op: event.target.value })}
              >
                <option value="gt">大于</option>
                <option value="gte">大于等于</option>
                <option value="eq">等于</option>
                <option value="lt">小于</option>
              </select>
            </label>
            <label className="xc-field">
              <span>目标值</span>
              <input
                className="xc-input"
                value={String(trigger.value ?? "")}
                onChange={(event) =>
                  setTrigger({
                    value: Number.isNaN(Number(event.target.value))
                      ? event.target.value
                      : Number(event.target.value),
                  })
                }
                placeholder="90"
              />
            </label>
          </>
        )}
        {trigger.type === "ha_state" && (
          <>
            <label className="xc-field">
              <span>实体 ID</span>
              <input
                className="xc-input"
                value={trigger.entityId ?? ""}
                onChange={(event) =>
                  setTrigger({ entityId: event.target.value })
                }
                required
              />
            </label>
            <label className="xc-field">
              <span>目标状态</span>
              <input
                className="xc-input"
                value={trigger.state ?? ""}
                onChange={(event) => setTrigger({ state: event.target.value })}
                required
              />
            </label>
          </>
        )}
        {trigger.type === "webhook" && (
          <p className="xc-muted">
            保存后会生成独立 URL。调用方需要保存完整地址。
          </p>
        )}
      </div>
      <h3>条件</h3>
      {form.conditions.map((item, index) => (
        <div className="automation-row" key={index}>
          <input
            className="xc-input"
            aria-label="字段路径"
            placeholder="data.hostId"
            value={item.field}
            onChange={(event) =>
              setCondition(index, { field: event.target.value })
            }
          />
          <select
            className="xc-select"
            value={item.op}
            onChange={(event) =>
              setCondition(index, { op: event.target.value as Condition["op"] })
            }
          >
            {["eq", "ne", "gt", "gte", "lt", "lte", "contains"].map((op) => (
              <option key={op} value={op}>
                {op}
              </option>
            ))}
          </select>
          <input
            className="xc-input"
            aria-label="比较值"
            value={String(item.value)}
            onChange={(event) =>
              setCondition(index, {
                value: Number.isNaN(Number(event.target.value))
                  ? event.target.value
                  : Number(event.target.value),
              })
            }
          />
          <button
            type="button"
            className="xc-btn small"
            onClick={() =>
              setForm({
                ...form,
                conditions: form.conditions.filter((_, i) => i !== index),
              })
            }
          >
            移除
          </button>
        </div>
      ))}
      <button
        type="button"
        className="xc-btn small"
        onClick={() =>
          setForm({
            ...form,
            conditions: [
              ...form.conditions,
              { field: "", op: "eq", value: "" },
            ],
          })
        }
      >
        添加条件
      </button>
      <h3>动作</h3>
      {form.actions.map((step, index) => (
        <div className="automation-step" key={index}>
          <div className="automation-row">
            <select
              className="xc-select"
              aria-label="动作"
              value={step.action}
              onChange={(event) =>
                setStep(index, { action: event.target.value, input: {} })
              }
            >
              <option value="">选择动作</option>
              {catalog.map((item) => (
                <option key={item.name} value={item.name}>
                  {item.title} · {item.name}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="xc-btn small"
              onClick={() =>
                setForm({
                  ...form,
                  actions: form.actions.filter((_, i) => i !== index),
                })
              }
            >
              移除
            </button>
          </div>
          {step.action && (
            <ActionInput
              step={step}
              catalog={catalog}
              change={(input) => setStep(index, { input })}
            />
          )}
        </div>
      ))}
      <button
        type="button"
        className="xc-btn small"
        onClick={() =>
          setForm({
            ...form,
            actions: [...form.actions, { action: "", input: {} }],
          })
        }
      >
        添加动作
      </button>
      <div className="xc-row automation-actions">
        <button className="xc-btn primary" disabled={save.isPending}>
          保存规则
        </button>
        <button type="button" className="xc-btn" onClick={close}>
          取消
        </button>
      </div>
    </form>
  );
}

export default function AutomationsPage() {
  const rules = useAutomations();
  const catalog = useAutomationCatalog();
  const [selected, setSelected] = useState("");
  const [editing, setEditing] = useState(false);
  const [webhookUrl, setWebhookUrl] = useState("");
  const [deleteArmed, setDeleteArmed] = useState(false);
  const runs = useAutomationRuns(selected);
  const run = useRunAutomation();
  const remove = useDeleteAutomation();
  useEffect(() => {
    if (!selected && rules.data?.length) setSelected(rules.data[0].id);
  }, [rules.data, selected]);
  const active = rules.data?.find((item) => item.id === selected);
  const runNow = async () => {
    if (!selected) return;
    try {
      await withElevation(() => run.mutateAsync(selected));
      toast("已开始运行");
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  const deleteNow = async () => {
    if (!selected) return;
    try {
      await remove.mutateAsync(selected);
      setSelected("");
      setDeleteArmed(false);
      toast("规则已删除");
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };
  return (
    <div className="xc-page automation-page">
      <PageHeading
        title="自动化"
        aside={
          <button
            className="xc-btn primary"
            onClick={() => {
              setSelected("");
              setEditing(true);
              setWebhookUrl("");
            }}
          >
            <Plus size={14} /> 新建规则
          </button>
        }
      />
      {rules.isPending && <Loading />}
      {rules.isError && (
        <ErrorState error={rules.error} onRetry={() => rules.refetch()} />
      )}
      {catalog.isError && (
        <ErrorState error={catalog.error} onRetry={() => catalog.refetch()} />
      )}
      <div className="automation-layout">
        <aside className="xc-card automation-list">
          {rules.data?.length === 0 && (
            <p className="xc-muted">还没有自动化规则。</p>
          )}
          {rules.data?.map((item) => (
            <button
              key={item.id}
              className={item.id === selected ? "active" : ""}
              onClick={() => {
                setSelected(item.id);
                setEditing(false);
                setWebhookUrl("");
                setDeleteArmed(false);
              }}
            >
              <strong>{item.name}</strong>
              <small>
                {item.enabled ? "已启用" : "已停用"} · {item.trigger.type}
              </small>
            </button>
          ))}
        </aside>
        <section className="automation-detail">
          {editing ? (
            <RuleEditor
              key={selected || "new"}
              rule={active}
              catalog={catalog.data?.actions ?? []}
              close={() => setEditing(false)}
              saved={(item) => {
                setSelected(item.id);
                setWebhookUrl(item.webhookUrl ?? "");
                setEditing(false);
              }}
            />
          ) : active ? (
            <>
              <div className="xc-card automation-summary">
                <h2>{active.name}</h2>
                <p>
                  触发：{active.trigger.type} · 冷却 {active.cooldownSeconds} 秒
                </p>
                {(webhookUrl || active.webhookUrl) && (
                  <label className="xc-field">
                    <span>Webhook URL</span>
                    <input
                      className="xc-input"
                      readOnly
                      value={`${window.location.origin}${webhookUrl || active.webhookUrl}`}
                      onFocus={(event) => event.target.select()}
                    />
                  </label>
                )}
                <div className="xc-row">
                  <button
                    className="xc-btn small"
                    onClick={() => setEditing(true)}
                  >
                    编辑
                  </button>
                  <button
                    className="xc-btn small"
                    onClick={() => void runNow()}
                    disabled={run.isPending}
                  >
                    <Play size={14} /> 手动运行
                  </button>
                  {deleteArmed ? (
                    <>
                      <button
                        className="xc-btn small danger"
                        onClick={() => void deleteNow()}
                      >
                        确认删除
                      </button>
                      <button
                        className="xc-btn small"
                        onClick={() => setDeleteArmed(false)}
                      >
                        取消
                      </button>
                    </>
                  ) : (
                    <button
                      className="xc-btn small danger"
                      onClick={() => setDeleteArmed(true)}
                    >
                      <Trash2 size={14} /> 删除
                    </button>
                  )}
                </div>
              </div>
              <h2>运行记录</h2>
              {runs.isPending && <Loading />}
              {runs.isError && (
                <ErrorState error={runs.error} onRetry={() => runs.refetch()} />
              )}
              {runs.data?.length === 0 && (
                <p className="xc-muted">还没有运行记录。</p>
              )}
              {runs.data?.map((item) => (
                <details className="xc-card automation-run" key={item.id}>
                  <summary>
                    {new Date(item.startedAt).toLocaleString()} · {item.status}
                  </summary>
                  <pre>{JSON.stringify(item.steps, null, 2)}</pre>
                </details>
              ))}
            </>
          ) : (
            <p className="xc-muted">选择规则或新建规则。</p>
          )}
        </section>
      </div>
    </div>
  );
}
