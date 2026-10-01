import { useState } from "react";
import { Plus } from "lucide-react";
import Dialog from "../../components/ui/Dialog";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { CopyButton } from "../../components/ui/ErrorNotices";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { errorMessage } from "../../api/client";
import {
  useCalls,
  useTokenMutations,
  useTokens,
  useTools,
  type ApiToken,
  type ApiTokenAccess,
} from "./api";
import "./i18n";
import "./mcp.css";

const ACCESS: { value: ApiTokenAccess; label: string; hint: string }[] = [
  {
    value: "read",
    label: "Read only",
    hint: "Search and read. Changes nothing.",
  },
  {
    value: "write",
    label: "Read and write",
    hint: "Create and change notes, cards, reminders. Cannot delete.",
  },
  {
    value: "write_delete",
    label: "Read, write and delete",
    hint: "Can also delete. Use only for clients you trust.",
  },
];

const EXPIRY = [
  { days: 30, label: "30 days" },
  { days: 90, label: "90 days" },
  { days: 365, label: "1 year" },
  { days: 0, label: "No expiry" },
];

/** MCP 地址：用户打开页面的地址加上 /api/v1/mcp。 */
export function mcpUrl() {
  return `${location.origin}/api/v1/mcp`;
}

/** 三种客户端的接入写法（B43）。 */
export function setupSnippets(url: string, secret: string) {
  return {
    claude: `claude mcp add --transport http xconsole ${url} --header "Authorization: Bearer ${secret}"`,
    codex: `# ~/.codex/config.toml\n[mcp_servers.xconsole]\nurl = "${url}"\nbearer_token_env_var = "XCONSOLE_TOKEN"\n\n# 再在 shell 里设置：\nexport XCONSOLE_TOKEN=${secret}`,
    json: JSON.stringify(
      {
        mcpServers: {
          xconsole: {
            url,
            headers: { Authorization: `Bearer ${secret}` },
          },
        },
      },
      null,
      2,
    ),
  };
}

/** 设置 → 远程访问（B43）：API 令牌、接入说明、最近调用。 */
export default function RemoteAccessTab() {
  const t = useT();
  const tokens = useTokens();
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<string | null>(null);
  if (tokens.isPending) return <Loading />;
  if (tokens.isError)
    return <ErrorState error={tokens.error} onRetry={() => tokens.refetch()} />;
  const placeholder = "xc_<令牌>";
  const snippets = setupSnippets(mcpUrl(), created ?? placeholder);
  return (
    <div className="settings-grid mcp-settings">
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("API tokens")}</h2>
          <button
            className="xc-btn primary small"
            onClick={() => setCreating(true)}
          >
            <Plus size={14} /> {t("New token")}
          </button>
        </div>
        <p className="settings-note">
          {t(
            "Other AI clients (Claude Code, Codex, Cursor…) use a token to read and change your data through MCP. Running commands, power and settings are never available.",
          )}
        </p>
        <TokenList tokens={tokens.data} />
      </section>
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("How to connect")}</h2>
        </div>
        <p className="xc-muted">
          {t("Address")}: <code className="xc-mono">{mcpUrl()}</code>
        </p>
        <Snippet title="Claude Code" text={snippets.claude} />
        <Snippet title="Codex CLI" text={snippets.codex} />
        <Snippet
          title={t("Cursor and other JSON configs")}
          text={snippets.json}
        />
        <small className="xc-muted">
          {t(
            "Cloud AIs (claude.ai, ChatGPT) need the address reachable from the internet and an OAuth login, which is not supported yet.",
          )}
        </small>
      </section>
      <Calls />
      <CreateDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(secret) => {
          setCreating(false);
          setCreated(secret);
        }}
      />
      <Dialog
        open={created !== null}
        onClose={() => setCreated(null)}
        title={t("Token created")}
      >
        <p className="mcp-warning">
          {t(
            "Copy it now. After closing this window it cannot be shown again.",
          )}
        </p>
        <div className="mcp-secret">
          <code className="xc-mono">{created}</code>
          {created && <CopyButton text={created} />}
        </div>
        <Snippet title="Claude Code" text={snippets.claude} />
        <div className="xc-dialog-actions">
          <button className="xc-btn primary" onClick={() => setCreated(null)}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    </div>
  );
}

function Snippet({ title, text }: { title: string; text: string }) {
  return (
    <div className="mcp-snippet">
      <div className="mcp-snippet-head">
        <strong>{title}</strong>
        <CopyButton text={text} />
      </div>
      <pre className="xc-mono">{text}</pre>
    </div>
  );
}

function TokenList({ tokens }: { tokens: ApiToken[] }) {
  const t = useT();
  const language = useLanguage();
  const ops = useTokenMutations();
  if (tokens.length === 0)
    return <p className="xc-muted">{t("No tokens yet.")}</p>;
  const access = (a: ApiTokenAccess) =>
    t(ACCESS.find((x) => x.value === a)?.label ?? a);
  return (
    <ul className="mcp-tokens">
      {tokens.map((tok) => {
        const expired =
          !!tok.expiresAt && new Date(tok.expiresAt).getTime() < Date.now();
        const dead = !!tok.revokedAt || expired;
        return (
          <li key={tok.id} className={dead ? "is-dead" : ""}>
            <div>
              <strong>{tok.name}</strong>
              <span className="xc-mono mcp-prefix">{tok.prefix}…</span>
              <span className="xc-badge">{access(tok.access)}</span>
              {tok.revokedAt && (
                <span className="xc-badge danger">{t("Revoked")}</span>
              )}
              {expired && !tok.revokedAt && (
                <span className="xc-badge warn">{t("Expired")}</span>
              )}
            </div>
            <small className="xc-muted">
              {tok.modules.length ? tok.modules.join("、") : t("All modules")}
              {" · "}
              {tok.lastUsedAt
                ? `${t("Last used")} ${relativeTime(tok.lastUsedAt, language)}${tok.lastUsedIp ? ` (${tok.lastUsedIp})` : ""}`
                : t("Never used")}
              {" · "}
              {tok.expiresAt
                ? `${t("Expiry")} ${relativeTime(tok.expiresAt, language)}`
                : t("Does not expire")}
            </small>
            {!tok.revokedAt && (
              <button
                className="xc-btn danger small"
                disabled={ops.revoke.isPending}
                onClick={async () => {
                  if (
                    await confirmAction({
                      title: `${t("Revoke token")}“${tok.name}”？`,
                      description: t(
                        "Clients using it stop working at once. This cannot be undone.",
                      ),
                      confirmLabel: t("Revoke"),
                    })
                  )
                    ops.revoke.mutate(tok.id);
                }}
              >
                {t("Revoke")}
              </button>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function CreateDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (secret: string) => void;
}) {
  const t = useT();
  const ops = useTokenMutations();
  const [name, setName] = useState("");
  const [access, setAccess] = useState<ApiTokenAccess>("write");
  const [modules, setModules] = useState<string[]>([]);
  const [days, setDays] = useState(90);
  const [error, setError] = useState("");
  const tools = useTools(access, modules);
  const allModules = tools.data?.modules ?? [];
  return (
    <Dialog open={open} onClose={onClose} title={t("New token")} wide>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setError("");
          ops.create.mutate(
            {
              name: name.trim(),
              access,
              modules,
              expiresInDays: days || undefined,
            },
            {
              onSuccess: (r) => {
                setName("");
                onCreated(r.secret);
              },
              onError: (err) => setError(errorMessage(err)),
            },
          );
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={60}
            placeholder={t("For example: Claude Code on my laptop")}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
        </label>
        <div className="xc-field">
          <span>{t("Access")}</span>
          <div className="mcp-access">
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
                  <small>{t(a.hint)}</small>
                </span>
              </label>
            ))}
          </div>
        </div>
        <div className="xc-field">
          <span>{t("Modules")}</span>
          <div className="mcp-modules">
            {allModules.map((m) => {
              const on = modules.includes(m);
              return (
                <button
                  type="button"
                  key={m}
                  className={`xc-btn small${on ? " on" : ""}`}
                  aria-pressed={on}
                  onClick={() =>
                    setModules(
                      on ? modules.filter((x) => x !== m) : [...modules, m],
                    )
                  }
                >
                  {m}
                </button>
              );
            })}
          </div>
          <small>
            {modules.length
              ? t("Only the picked modules.")
              : t("None picked means all modules.")}{" "}
            {tools.data &&
              t("{n} tools available.").replace(
                "{n}",
                String(tools.data.tools.length),
              )}
          </small>
        </div>
        <label className="xc-field">
          <span>{t("Expires after")}</span>
          <select
            className="xc-select"
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          >
            {EXPIRY.map((x) => (
              <option key={x.days} value={x.days}>
                {t(x.label)}
              </option>
            ))}
          </select>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!name.trim() || ops.create.isPending}
          >
            {t("Create token")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function Calls() {
  const t = useT();
  const language = useLanguage();
  const calls = useCalls();
  return (
    <section className="xc-card mcp-calls-card">
      <div className="xc-card-head">
        <h2>{t("Recent calls")}</h2>
      </div>
      {!calls.data?.length ? (
        <p className="xc-muted">{t("No calls yet.")}</p>
      ) : (
        <ul className="mcp-calls">
          {calls.data.map((c) => (
            <li key={c.id}>
              <span className="xc-mono">{c.tool}</span>
              <span className="xc-muted">{c.token}</span>
              <span
                className={c.result === "ok" ? "ok" : "err"}
                title={c.result}
              >
                {c.result === "ok"
                  ? t("OK")
                  : c.result.replace(/^error: ([a-z_]+: )?/, "")}
              </span>
              <time dateTime={c.at}>{relativeTime(c.at, language)}</time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
