import { useEffect, useState } from "react";
import { useT } from "../../../contexts/LanguageContext";
import { schemaFields, type Field } from "../logic";

/*
 * 动作的输入。能从 JSON Schema 生成简单表单就用表单，不能就用 JSON 编辑框。
 * 用户也可以手动切到 JSON。字符串里可以写 {{trigger.data.xxx}}。
 */
export default function StepInput({
  schema,
  value,
  onChange,
}: {
  schema: unknown;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
}) {
  const t = useT();
  const fields = schemaFields(schema);
  const [raw, setRaw] = useState(fields == null);
  useEffect(() => setRaw(fields == null), [schema]);

  return (
    <div className="auto-step-input">
      {fields && (
        <div className="auto-mode">
          <button
            type="button"
            className={raw ? "" : "on"}
            onClick={() => setRaw(false)}
          >
            {t("Form")}
          </button>
          <button
            type="button"
            className={raw ? "on" : ""}
            onClick={() => setRaw(true)}
          >
            JSON
          </button>
        </div>
      )}
      {raw || !fields ? (
        <JsonInput value={value} onChange={onChange} />
      ) : fields.length === 0 ? (
        <p className="auto-muted">{t("No parameters")}</p>
      ) : (
        fields.map((f) => (
          <FieldInput
            key={f.name}
            field={f}
            value={value[f.name]}
            onChange={(v) => onChange({ ...value, [f.name]: v })}
          />
        ))
      )}
    </div>
  );
}

function FieldInput({
  field,
  value,
  onChange,
}: {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const label = (
    <span>
      {field.label}
      {field.required && <em className="auto-required">*</em>}
    </span>
  );
  if (field.kind === "boolean")
    return (
      <label className="xc-check">
        <input
          type="checkbox"
          checked={value === true}
          onChange={(e) => onChange(e.target.checked)}
        />
        {label}
      </label>
    );
  if (field.kind === "enum")
    return (
      <label className="xc-field">
        {label}
        <select
          className="xc-select"
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(e.target.value)}
        >
          <option value="">—</option>
          {field.options!.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      </label>
    );
  const text = value == null ? "" : String(value);
  if (field.kind === "text")
    return (
      <label className="xc-field">
        {label}
        <textarea
          className="xc-textarea"
          rows={3}
          value={text}
          onChange={(e) => onChange(e.target.value)}
        />
      </label>
    );
  return (
    <label className="xc-field">
      {label}
      <input
        className="xc-input"
        inputMode={field.kind === "string" ? undefined : "decimal"}
        value={text}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  );
}

function JsonInput({
  value,
  onChange,
}: {
  value: Record<string, unknown>;
  onChange: (v: Record<string, unknown>) => void;
}) {
  const [text, setText] = useState(() => JSON.stringify(value, null, 2));
  const [error, setError] = useState("");
  return (
    <label className="xc-field">
      <span>JSON</span>
      <textarea
        className="xc-textarea xc-mono"
        rows={6}
        spellCheck={false}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          try {
            const parsed = JSON.parse(e.target.value || "{}");
            if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
              throw new Error("要是一个对象");
            setError("");
            onChange(parsed);
          } catch (err) {
            setError(err instanceof Error ? err.message : String(err));
          }
        }}
      />
      {error && <small className="auto-danger">JSON 有误：{error}</small>}
    </label>
  );
}
