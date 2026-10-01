import { useState } from "react";
import {
  GitBranch,
  KeyRound,
  Plus,
  RefreshCw,
  Trash2,
  Webhook,
} from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import Dialog from "../../components/ui/Dialog";
import MoreMenu from "../../components/ui/MoreMenu";
import { Toolbar } from "../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { CopyButton } from "../../components/ui/ErrorNotices";
import { errorMessage } from "../../api/client";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { toast } from "../../hooks/useToast";
import {
  useConnectionMutations,
  useConnections,
  type GitConnection,
  type NewConnection,
} from "./api";
import AgentTabs from "./AgentTabs";
import "./i18n";
import "./aiagents.css";

/** /coding/connections：GitHub 和 Forgejo 连接（B47）。 */
export default function ConnectionsPage() {
  const t = useT();
  const list = useConnections();
  const [creating, setCreating] = useState(false);
  const [hook, setHook] = useState<{ url: string; secret: string } | null>(
    null,
  );
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Git connections")}
        subtitle={t("Agents clone, push and open pull requests through these.")}
        aside={
          <button className="xc-btn primary" onClick={() => setCreating(true)}>
            <Plus size={14} /> {t("New connection")}
          </button>
        }
      />
      <Toolbar start={<AgentTabs />} />
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 ? (
        <EmptyState
          title={t("No connections yet")}
          icon={<GitBranch size={26} />}
        >
          <span className="xc-muted">
            {t(
              "Connect GitHub or your own Forgejo, then add repositories from it.",
            )}
          </span>
          <button className="xc-btn small" onClick={() => setCreating(true)}>
            <Plus size={14} /> {t("New connection")}
          </button>
        </EmptyState>
      ) : (
        <div className="aiagents-grid">
          {list.data.map((c) => (
            <ConnectionCard key={c.id} conn={c} onWebhook={setHook} />
          ))}
        </div>
      )}
      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreated={(c, secret) => {
            setCreating(false);
            setHook({
              url: `${location.origin}/api/v1${c.webhookPath}`,
              secret,
            });
          }}
        />
      )}
      {hook && (
        <Dialog open onClose={() => setHook(null)} title={t("Webhook")}>
          <p className="xc-muted">
            {t(
              "Add a webhook for pull requests in the repository settings, so merged pull requests move their cards to done. The content type is application/json.",
            )}
          </p>
          <div className="aiagent-secret">
            <span>{t("Address")}</span>
            <code className="xc-mono">{hook.url}</code>
            <CopyButton text={hook.url} />
          </div>
          <div className="aiagent-secret">
            <span>{t("Secret")}</span>
            <code className="xc-mono">{hook.secret}</code>
            <CopyButton text={hook.secret} />
          </div>
          <div className="xc-dialog-actions">
            <button className="xc-btn primary" onClick={() => setHook(null)}>
              {t("Close")}
            </button>
          </div>
        </Dialog>
      )}
    </div>
  );
}

function ConnectionCard({
  conn: c,
  onWebhook,
}: {
  conn: GitConnection;
  onWebhook: (h: { url: string; secret: string }) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const ops = useConnectionMutations();
  const [tokenOpen, setTokenOpen] = useState(false);
  return (
    <div className="xc-card aiagent-card">
      <div className="aiagent-card-head">
        <span className="aiagent-conn-icon">
          <GitBranch size={18} />
        </span>
        <div className="aiagent-card-name">
          <strong>{c.name}</strong>
          <small className="xc-mono">{c.baseUrl}</small>
        </div>
        <MoreMenu
          label={`${t("More")}: ${c.name}`}
          title={c.name}
          items={[
            {
              key: "check",
              label: t("Check again"),
              icon: <RefreshCw size={14} />,
              onSelect: () => ops.check.mutate(c.id),
            },
            ...(c.useGithubModule
              ? []
              : [
                  {
                    key: "token",
                    label: t("Change token"),
                    icon: <KeyRound size={14} />,
                    onSelect: () => setTokenOpen(true),
                  },
                ]),
            {
              key: "hook",
              label: t("Webhook"),
              icon: <Webhook size={14} />,
              onSelect: async () => {
                const h = await ops.webhook.mutateAsync(c.id);
                onWebhook({
                  url: `${location.origin}/api/v1${h.path}`,
                  secret: h.secret,
                });
              },
            },
            {
              key: "delete",
              label: t("Delete"),
              icon: <Trash2 size={14} />,
              danger: true,
              onSelect: async () => {
                if (
                  await confirmAction({
                    title: `${t("Delete connection")}“${c.name}”？`,
                    description: t(
                      "Repositories added from it stay, but can no longer be cloned or open pull requests.",
                    ),
                    confirmLabel: t("Delete"),
                  })
                )
                  ops.remove.mutate(c.id);
              },
            },
          ]}
        />
      </div>
      <div className="aiagent-card-foot">
        <span className="xc-badge">
          {c.kind === "github" ? "GitHub" : "Forgejo / Gitea"}
        </span>
        {c.lastError ? (
          <span className="xc-badge danger" title={c.lastError}>
            {t("Check failed")}
          </span>
        ) : c.username ? (
          <span className="xc-badge ok">{c.username}</span>
        ) : null}
        {c.useGithubModule && (
          <span className="xc-muted">
            {t("Uses the GitHub module's token")}
          </span>
        )}
        {c.lastCheckedAt && (
          <span className="aiagent-card-cost">
            {t("Checked")} {relativeTime(c.lastCheckedAt, language)}
          </span>
        )}
      </div>
      {c.lastError && <p className="aiagent-card-error">{c.lastError}</p>}
      {tokenOpen && (
        <TokenDialog conn={c} onClose={() => setTokenOpen(false)} />
      )}
    </div>
  );
}

function TokenDialog({
  conn,
  onClose,
}: {
  conn: GitConnection;
  onClose: () => void;
}) {
  const t = useT();
  const ops = useConnectionMutations();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  return (
    <Dialog open onClose={onClose} title={`${t("Change token")}：${conn.name}`}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            const c = await ops.update.mutateAsync({
              id: conn.id,
              body: { token },
            });
            toast(c.lastError ? t("Saved, but the check failed") : t("Saved"));
            onClose();
          } catch (err) {
            setError(errorMessage(err));
          }
        }}
      >
        <label className="xc-field">
          <span>{t("Access token")}</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="off"
            value={token}
            required
            autoFocus
            onChange={(e) => setToken(e.target.value)}
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!token || ops.update.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function CreateDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (c: GitConnection, webhookSecret: string) => void;
}) {
  const t = useT();
  const ops = useConnectionMutations();
  const [kind, setKind] = useState<NewConnection["kind"]>("github");
  const [name, setName] = useState("GitHub");
  const [baseUrl, setBaseUrl] = useState("");
  const [token, setToken] = useState("");
  const [shared, setShared] = useState(false);
  const [error, setError] = useState("");
  const github = kind === "github";
  return (
    <Dialog open onClose={onClose} title={t("New connection")} wide>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setError("");
          try {
            const body: NewConnection = { kind, name: name.trim() };
            if (baseUrl.trim()) body.baseUrl = baseUrl.trim();
            if (github && shared) body.useGithubModule = true;
            else body.token = token.trim();
            const r = await ops.create.mutateAsync(body);
            toast(
              r.connection.lastError
                ? t("Saved, but the check failed")
                : t("Saved"),
            );
            onCreated(r.connection, r.webhookSecret);
          } catch (err) {
            setError(errorMessage(err));
          }
        }}
      >
        <div className="xc-field">
          <span>{t("Type")}</span>
          <div className="aiagent-kinds aiagent-kinds-row">
            {(["github", "forgejo"] as const).map((k) => (
              <label key={k} className="elevation-mode">
                <input
                  type="radio"
                  name="kind"
                  checked={kind === k}
                  onChange={() => {
                    setKind(k);
                    setName(k === "github" ? "GitHub" : "Forgejo");
                  }}
                />
                <span>
                  <strong>
                    {k === "github" ? "GitHub" : "Forgejo / Gitea"}
                  </strong>
                </span>
              </label>
            ))}
          </div>
        </div>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={60}
            required
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Address")}</span>
          <input
            className="xc-input"
            value={baseUrl}
            required={!github}
            placeholder={
              github ? "https://api.github.com" : "https://git.example.com"
            }
            onChange={(e) => setBaseUrl(e.target.value)}
          />
          <small>
            {github
              ? t(
                  "Leave empty for github.com. GitHub Enterprise: its API address.",
                )
              : t("The address you open Forgejo with.")}
          </small>
        </label>
        {github && (
          <label className="xc-check">
            <input
              type="checkbox"
              checked={shared}
              onChange={(e) => setShared(e.target.checked)}
            />
            {t("Use the token from the GitHub settings")}
          </label>
        )}
        {!(github && shared) && (
          <label className="xc-field">
            <span>{t("Access token")}</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="off"
              value={token}
              required
              onChange={(e) => setToken(e.target.value)}
            />
            <small>
              {github
                ? t(
                    "Fine-grained token with read and write access to Contents and Pull requests.",
                  )
                : t("A token with the write:repository scope.")}
            </small>
          </label>
        )}
        <small className="xc-muted">
          {t("The token is stored encrypted and never shown again.")}
        </small>
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
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
