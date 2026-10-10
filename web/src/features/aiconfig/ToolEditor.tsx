import { Plus, Trash2 } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { TOOL_LABELS, emptyMcp, type McpDraft, type ToolDraft } from "./format";

/** Claude Code 或 Codex 的一份配置：规则、权限（只有 Claude Code）、MCP 服务器。 */
export default function ToolEditor({
  tool,
  value,
  onChange,
}: {
  tool: "claude" | "codex";
  value: ToolDraft;
  onChange: (next: ToolDraft) => void;
}) {
  const t = useT();
  const set = (patch: Partial<ToolDraft>) => onChange({ ...value, ...patch });
  const label = TOOL_LABELS[tool];
  const rulesHint =
    tool === "claude"
      ? "Written to CLAUDE.md in the Claude Code folder, between markers. Text outside the markers is not touched."
      : "Written to AGENTS.md in the Codex folder, between markers. Text outside the markers is not touched.";
  return (
    <section className="xc-card aiconfig-tool" aria-label={label}>
      <div className="xc-card-head">
        <h2>{label}</h2>
      </div>
      <div className="aiconfig-body">
        <div className="xc-field">
          <label htmlFor={`rules-${tool}`}>{t("Global rules")}</label>
          <textarea
            id={`rules-${tool}`}
            className="xc-textarea aiconfig-mono"
            rows={8}
            value={value.rules}
            spellCheck={false}
            onChange={(e) => set({ rules: e.target.value })}
          />
          <small>{t(rulesHint)}</small>
        </div>
        {tool === "claude" && (
          <div className="aiconfig-perms">
            <div className="aiconfig-perm-grid">
              {(
                [
                  ["allow", "Allow"],
                  ["ask", "Ask first"],
                  ["deny", "Deny"],
                ] as const
              ).map(([key, title]) => (
                <div className="xc-field" key={key}>
                  <label htmlFor={`perm-${key}`}>{t(title)}</label>
                  <textarea
                    id={`perm-${key}`}
                    className="xc-textarea aiconfig-mono"
                    rows={5}
                    value={value[key]}
                    spellCheck={false}
                    onChange={(e) => set({ [key]: e.target.value })}
                  />
                </div>
              ))}
            </div>
            <small className="aiconfig-hint">
              {t(
                "One rule per line, for example Bash(git status). Added to the lists in settings.json. Rules you wrote there yourself are kept.",
              )}
            </small>
          </div>
        )}
        <McpEditor
          servers={value.mcp}
          onChange={(mcp) => set({ mcp })}
          prefix={tool}
        />
      </div>
    </section>
  );
}

function McpEditor({
  servers,
  onChange,
  prefix,
}: {
  servers: McpDraft[];
  onChange: (next: McpDraft[]) => void;
  prefix: string;
}) {
  const t = useT();
  const patch = (i: number, p: Partial<McpDraft>) =>
    onChange(servers.map((s, j) => (j === i ? { ...s, ...p } : s)));
  return (
    <div className="aiconfig-mcp">
      <div className="aiconfig-mcp-head">
        <strong>{t("MCP servers")}</strong>
        <button
          type="button"
          className="xc-btn small"
          onClick={() => onChange([...servers, emptyMcp()])}
        >
          <Plus size={14} /> {t("Add server")}
        </button>
      </div>
      {servers.length === 0 && (
        <small className="aiconfig-hint">{t("No servers.")}</small>
      )}
      {servers.map((s, i) => (
        <div className="aiconfig-server" key={i}>
          <div className="aiconfig-server-row">
            <div className="xc-field">
              <label htmlFor={`${prefix}-mcp-name-${i}`}>
                {t("Server name")}
              </label>
              <input
                id={`${prefix}-mcp-name-${i}`}
                className="xc-input"
                value={s.name}
                maxLength={40}
                autoComplete="off"
                onChange={(e) => patch(i, { name: e.target.value })}
              />
            </div>
            <div className="xc-field aiconfig-narrow">
              <label htmlFor={`${prefix}-mcp-kind-${i}`}>
                {t("Transport")}
              </label>
              <select
                id={`${prefix}-mcp-kind-${i}`}
                className="xc-select"
                value={s.transport}
                onChange={(e) =>
                  patch(i, {
                    transport: e.target.value as McpDraft["transport"],
                  })
                }
              >
                <option value="stdio">{t("Local process")}</option>
                <option value="http">{t("Remote (HTTP)")}</option>
              </select>
            </div>
            <button
              type="button"
              className="xc-btn ghost danger small aiconfig-remove"
              aria-label={`${t("Remove server")} ${s.name}`}
              title={t("Remove server")}
              onClick={() => onChange(servers.filter((_, j) => j !== i))}
            >
              <Trash2 size={14} />
            </button>
          </div>
          {s.transport === "http" ? (
            <div className="xc-field">
              <label htmlFor={`${prefix}-mcp-url-${i}`}>{t("Address")}</label>
              <input
                id={`${prefix}-mcp-url-${i}`}
                className="xc-input aiconfig-mono"
                value={s.url}
                placeholder="https://"
                autoComplete="off"
                onChange={(e) => patch(i, { url: e.target.value })}
              />
            </div>
          ) : (
            <div className="aiconfig-server-row">
              <div className="xc-field">
                <label htmlFor={`${prefix}-mcp-cmd-${i}`}>{t("Command")}</label>
                <input
                  id={`${prefix}-mcp-cmd-${i}`}
                  className="xc-input aiconfig-mono"
                  value={s.command}
                  placeholder="npx"
                  autoComplete="off"
                  onChange={(e) => patch(i, { command: e.target.value })}
                />
              </div>
              <div className="xc-field">
                <label htmlFor={`${prefix}-mcp-args-${i}`}>
                  {t("Arguments")}
                </label>
                <textarea
                  id={`${prefix}-mcp-args-${i}`}
                  className="xc-textarea aiconfig-mono"
                  rows={2}
                  value={s.args}
                  spellCheck={false}
                  onChange={(e) => patch(i, { args: e.target.value })}
                />
                <small>{t("One argument per line")}</small>
              </div>
            </div>
          )}
        </div>
      ))}
      {servers.length > 0 && (
        <small className="aiconfig-hint">
          {t(
            "Environment variables and headers are not supported. Set those on the machine.",
          )}
        </small>
      )}
    </div>
  );
}
