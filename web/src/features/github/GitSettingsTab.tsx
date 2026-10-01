import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router";
import {
  GitBranch,
  KeyRound,
  Plus,
  RefreshCw,
  Trash2,
  Webhook,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useConnectionMutations,
  useConnections,
  type GitConnection,
} from "../aiagents/api";
import {
  useGitHubConfig,
  useGitHubStatus,
  useSaveGitHubConfig,
  useSyncGitHub,
  type GitHubConfig,
} from "./api";
import GitAccountDialog, {
  TokenDialog,
  WebhookDialog,
} from "./GitAccountDialog";
import RepoPicker from "./RepoPicker";
import "../aiagents/aiagents.css";
import "./i18n";
import "./github.css";

/**
 * 设置 → Git 与 GitHub（B62）。Git 账号是唯一填令牌的地方：
 * Agent 用它克隆和提 PR，GitHub 页面选其中一个 GitHub 账号同步。
 */
export default function GitSettingsTab() {
  return (
    <div className="github-settings">
      <AccountsCard />
      <GitHubPageCard />
      <StatusCard />
    </div>
  );
}

function AccountsCard() {
  const t = useT();
  const list = useConnections();
  const [creating, setCreating] = useState(false);
  const [hook, setHook] = useState<{ url: string; secret: string } | null>(
    null,
  );
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Git accounts")}</h2>
        <button className="xc-btn small" onClick={() => setCreating(true)}>
          <Plus size={14} /> {t("New Git account")}
        </button>
      </div>
      <p className="xc-muted">
        {t(
          "GitHub and Forgejo accounts. Agents clone and open pull requests with them, and the GitHub page syncs with one of them.",
        )}
      </p>
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 ? (
        <p className="xc-muted">{t("No Git accounts yet.")}</p>
      ) : (
        <ul className="xc-list git-accounts">
          {list.data.map((c) => (
            <AccountRow key={c.id} conn={c} onWebhook={setHook} />
          ))}
        </ul>
      )}
      {creating && (
        <GitAccountDialog
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
      {hook && <WebhookDialog hook={hook} onClose={() => setHook(null)} />}
    </section>
  );
}

function AccountRow({
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
    <li className="xc-list-row git-account-row">
      <span className="aiagent-conn-icon">
        <GitBranch size={16} />
      </span>
      <div className="git-account-main">
        <div className="git-account-name">
          <strong>{c.name}</strong>
          <span className="xc-badge">
            {c.kind === "github" ? "GitHub" : "Forgejo / Gitea"}
          </span>
          {c.username && !c.lastError && (
            <span className="xc-badge ok">{c.username}</span>
          )}
        </div>
        <small className="xc-mono xc-muted">{c.baseUrl}</small>
        {c.lastError ? (
          <small className="xc-error-text">{c.lastError}</small>
        ) : (
          c.lastCheckedAt && (
            <small className="xc-muted">
              {t("Checked")} {relativeTime(c.lastCheckedAt, language)}
            </small>
          )
        )}
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
          {
            key: "token",
            label: t("Change token"),
            icon: <KeyRound size={14} />,
            onSelect: () => setTokenOpen(true),
          },
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
                  title: `${t("Delete Git account")}“${c.name}”？`,
                  description: t(
                    "Repositories added from it stay, but can no longer be cloned or open pull requests.",
                  ),
                  confirmLabel: t("Delete"),
                })
              )
                ops.remove.mutate(c.id, {
                  onError: (error) =>
                    toast({ message: errorMessage(error), tone: "error" }),
                });
            },
          },
        ]}
      />
      {tokenOpen && (
        <TokenDialog conn={c} onClose={() => setTokenOpen(false)} />
      )}
    </li>
  );
}

function GitHubPageCard() {
  const t = useT();
  const config = useGitHubConfig();
  const accounts = useConnections();
  if (config.isPending || accounts.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  const github = (accounts.data ?? []).filter((c) => c.kind === "github");
  return <PageForm initial={config.data} accounts={github} />;
}

function PageForm({
  initial,
  accounts,
}: {
  initial: GitHubConfig;
  accounts: GitConnection[];
}) {
  const t = useT();
  const save = useSaveGitHubConfig();
  const sync = useSyncGitHub();
  const [repos, setRepos] = useState<string[]>(initial.repos);
  const [account, setAccount] = useState<number>(initial.connectionId ?? 0);
  useEffect(() => {
    setRepos(initial.repos);
    setAccount(initial.connectionId ?? 0);
  }, [initial]);

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      { repos, connectionId: account },
      {
        onSuccess: (cfg) => {
          toast(t("Saved"));
          // 保存后马上同步一次，页面上很快就有数据。
          if (cfg.hasToken) sync.mutate();
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("GitHub page")}</h2>
        <Link className="xc-btn small ghost" to="/github">
          {t("Open")}
        </Link>
      </div>
      {accounts.length === 0 ? (
        <p className="xc-muted">{t("Add a GitHub account above first.")}</p>
      ) : (
        <>
          <label className="xc-field">
            <span>{t("GitHub account")}</span>
            <select
              className="xc-select"
              value={account}
              onChange={(e) => setAccount(Number(e.target.value))}
            >
              <option value={0}>{t("Not selected")}</option>
              {accounts.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                  {c.username ? ` · ${c.username}` : ""}
                </option>
              ))}
            </select>
            {!initial.connectionId && initial.hasToken && (
              <small>
                {t(
                  "Using the token from the old GitHub settings. Pick an account to switch.",
                )}
              </small>
            )}
          </label>
          <div className="xc-field">
            <span>{t("Watched repositories")}</span>
            <RepoPicker
              key={`${initial.repos.join(",")}:${initial.connectionId ?? 0}`}
              value={repos}
              onChange={setRepos}
              hasToken={
                initial.hasToken && (initial.connectionId ?? 0) === account
              }
            />
          </div>
          <div className="xc-dialog-actions github-actions">
            <span className="xc-spacer" />
            <button className="xc-btn primary" disabled={save.isPending}>
              {t("Save")}
            </button>
          </div>
        </>
      )}
    </form>
  );
}

function StatusCard() {
  const t = useT();
  const language = useLanguage();
  const status = useGitHubStatus();
  const sync = useSyncGitHub();
  if (!status.data) return null;
  const s = status.data;
  const busy = sync.isPending || s.syncing;
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Sync status")}</h2>
        {s.configured && (
          <button
            className="xc-btn small"
            disabled={busy}
            onClick={() =>
              sync.mutate(undefined, {
                onError: (error) =>
                  toast({ message: errorMessage(error), tone: "error" }),
              })
            }
          >
            <RefreshCw size={14} className={busy ? "github-spin" : undefined} />
            {busy ? t("Syncing") : t("Sync now")}
          </button>
        )}
      </div>
      {!s.configured ? (
        <p className="xc-muted">{t("Not configured")}</p>
      ) : (
        <dl className="github-status">
          {s.login && (
            <>
              <dt>{t("Account")}</dt>
              <dd className="xc-mono">{s.login}</dd>
            </>
          )}
          <dt>{t("Watched repository count")}</dt>
          <dd>{s.repoCount}</dd>
          <dt>{t("Last sync")}</dt>
          <dd>
            {s.lastSyncAt ? (
              <>
                {relativeTime(s.lastSyncAt, language)}{" "}
                {s.lastError ? (
                  <span className="xc-badge danger">{t("Failed")}</span>
                ) : (
                  <span className="xc-badge ok">{t("OK")}</span>
                )}
              </>
            ) : (
              t("Never")
            )}
          </dd>
          {s.syncIntervalSeconds && (
            <>
              <dt>{t("Sync interval")}</dt>
              <dd>
                {s.syncIntervalSeconds >= 120
                  ? `${Math.round(s.syncIntervalSeconds / 60)} ${t("minutes")}`
                  : `${s.syncIntervalSeconds} ${t("seconds")}`}
                {s.syncIntervalSeconds >= 300 && (
                  <span className="xc-muted">
                    {" "}
                    · {t("Slowed down because few requests are left")}
                  </span>
                )}
              </dd>
            </>
          )}
          {s.lastError && (
            <>
              <dt>{t("Last error")}</dt>
              <dd className="xc-error-text">{s.lastError}</dd>
            </>
          )}
          {s.rateLimitRemaining != null && (
            <>
              <dt>{t("Requests left")}</dt>
              <dd>
                {s.rateLimitRemaining}
                {s.rateLimitLimit ? ` / ${s.rateLimitLimit}` : ""}
                {s.rateLimitResetAt && (
                  <span className="xc-muted">
                    {" "}
                    · {t("Resets")}：
                    {relativeTime(s.rateLimitResetAt, language)}
                  </span>
                )}
              </dd>
            </>
          )}
        </dl>
      )}
    </div>
  );
}
