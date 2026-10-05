import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router";
import { KeyRound, Plus, RefreshCw, Trash2, Webhook, X } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
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
  useGitHubNotify,
  useGitHubStatus,
  useSaveGitHubConfig,
  useSaveGitHubNotify,
  useSyncGitHub,
  type GitHubConfig,
  type RepoNotify,
  type RepoWatch,
} from "./api";
import { ForgeIcon, RepoSwatch } from "./RepoBits";
import { DEFAULT_NOTIFY } from "./logic";
import NotifyFields from "./NotifyFields";

/** 下拉框里的账号名：名字里没写类型时补上，比如“公司（Forgejo）· j0x3n”。 */
function accountLabel(c: GitConnection) {
  const kind = c.kind === "github" ? "GitHub" : "Forgejo";
  const name = c.name.toLowerCase().includes(kind.toLowerCase())
    ? c.name
    : `${c.name}（${kind}）`;
  return c.username ? `${name} · ${c.username}` : name;
}

/** B70：最多关注 100 个仓库，和接口的 maxItems 一致。 */
const MAX_WATCHES = 100;
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
  // B70：两栏。左边账号、同步状态、通知；右边关注的仓库。窄屏一栏。
  return (
    <div className="github-settings">
      <div className="github-settings-col">
        <AccountsCard />
        <StatusCard />
        <NotifyCard />
        <CIQuotaCard />
      </div>
      <div className="github-settings-col">
        <GitHubPageCard />
      </div>
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
          "GitHub and Forgejo accounts. Agents clone and open pull requests with them, and the repositories page syncs with them.",
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
        <ForgeIcon forge={c.kind} size={16} />
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
  const config = useGitHubConfig();
  const accounts = useConnections();
  if (config.isPending || accounts.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  // 后端返回了 watches 说明 B70 已上线：GitHub 和 Forgejo 的账号都能选仓库。
  if (config.data.watches)
    return <WatchForm initial={config.data} accounts={accounts.data ?? []} />;
  const github = (accounts.data ?? []).filter((c) => c.kind === "github");
  return <PageForm initial={config.data} accounts={github} />;
}

/** B70：关注的仓库，每个仓库属于一个 Git 账号（GitHub 或 Forgejo）。 */
function WatchForm({
  initial,
  accounts,
}: {
  initial: GitHubConfig;
  accounts: GitConnection[];
}) {
  const t = useT();
  const save = useSaveGitHubConfig();
  const sync = useSyncGitHub();
  const [watches, setWatches] = useState<RepoWatch[]>(initial.watches ?? []);
  const [account, setAccount] = useState<number>(
    () =>
      initial.watches?.[0]?.connectionId ??
      initial.connectionId ??
      accounts[0]?.id ??
      0,
  );
  useEffect(() => setWatches(initial.watches ?? []), [initial]);
  const current = accounts.find((a) => a.id === account);
  const names = watches
    .filter((w) => w.connectionId === account)
    .map((w) => w.repo);
  const setNames = (repos: string[]) =>
    setWatches((prev) => [
      ...prev.filter((w) => w.connectionId !== account),
      ...repos.map((repo) => ({ connectionId: account, repo })),
    ]);
  const remove = (w: RepoWatch) =>
    setWatches((prev) =>
      prev.filter(
        (x) => !(x.connectionId === w.connectionId && x.repo === w.repo),
      ),
    );
  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      { repos: watches.map((w) => w.repo), watches },
      {
        onSuccess: () => {
          toast(t("Saved"));
          // 保存后马上同步一次，页面上很快就有数据。
          sync.mutate();
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };
  const byAccount = accounts
    .map((a) => ({
      account: a,
      list: watches.filter((w) => w.connectionId === a.id),
    }))
    .filter((g) => g.list.length > 0);
  const orphans = watches.filter(
    (w) => !accounts.some((a) => a.id === w.connectionId),
  );
  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("Watched repositories")}</h2>
        <Link className="xc-btn small ghost" to="/github">
          {t("Open")}
        </Link>
      </div>
      {accounts.length === 0 ? (
        <p className="xc-muted">{t("Add a Git account first.")}</p>
      ) : (
        <>
          {byAccount.length === 0 && orphans.length === 0 ? (
            <p className="xc-muted github-picked-none">
              {t("No repositories yet. Pick some below.")}
            </p>
          ) : (
            <div className="git-watch-groups">
              {byAccount.map(({ account: a, list }) => (
                <div key={a.id} className="git-watch-group">
                  <div className="git-watch-account">
                    <ForgeIcon forge={a.kind} size={12} />
                    <span>{a.name}</span>
                    <small className="xc-muted">{list.length}</small>
                  </div>
                  <ul className="github-picked">
                    {list.map((w) => (
                      <li key={w.repo} className="xc-badge">
                        <RepoSwatch repo={w.repo} />
                        <span className="xc-mono">{w.repo}</span>
                        <button
                          type="button"
                          aria-label={`${t("Remove")} ${w.repo}`}
                          onClick={() => remove(w)}
                        >
                          <X size={12} />
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
              {orphans.length > 0 && (
                <small className="xc-error-text">
                  {t("Some repositories belong to a deleted account:")}{" "}
                  {orphans.map((w) => w.repo).join("、")}
                </small>
              )}
            </div>
          )}
          <label className="xc-field">
            <span>{t("Pick from account")}</span>
            <select
              className="xc-select"
              value={account}
              onChange={(e) => setAccount(Number(e.target.value))}
            >
              {accounts.map((c) => (
                <option key={c.id} value={c.id}>
                  {accountLabel(c)}
                </option>
              ))}
            </select>
          </label>
          {current && (
            <RepoPicker
              key={current.id}
              value={names}
              onChange={setNames}
              hasToken={current.hasToken}
              connectionId={current.id}
              max={MAX_WATCHES - (watches.length - names.length)}
              hidePicked
            />
          )}
          <div className="xc-dialog-actions github-actions">
            <small className="xc-muted">
              {watches.length} / {MAX_WATCHES}
            </small>
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

/** B71：所有仓库默认发哪些通知。单个仓库在仓库页的铃铛里单独设。 */
function NotifyCard() {
  const t = useT();
  const settings = useGitHubNotify();
  const save = useSaveGitHubNotify();
  const [value, setValue] = useState<RepoNotify>(DEFAULT_NOTIFY);
  const [ciQuota, setCiQuota] = useState(true);
  useEffect(() => {
    if (settings.data) {
      setValue(settings.data.defaults);
      setCiQuota(settings.data.ciQuota ?? true);
    }
  }, [settings.data]);
  if (settings.isError && isNotLive(settings.error))
    return (
      <section className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Repository notifications")}</h2>
        </div>
        <p className="xc-muted">
          {t("Repository notifications")}
          {t(" are not live yet.")}
        </p>
      </section>
    );
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Repository notifications")}</h2>
      </div>
      <p className="xc-muted">
        {t(
          "Default for all watched repositories. Use the bell on the repositories page to change one repository.",
        )}
      </p>
      {settings.isPending ? (
        <Loading />
      ) : settings.isError ? (
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      ) : (
        <>
          <NotifyFields value={value} onChange={setValue} />
          <label className="xc-check">
            <input
              type="checkbox"
              checked={ciQuota}
              onChange={(e) => setCiQuota(e.target.checked)}
            />
            <span>
              {t("Notify when this month's CI minutes reach 80% and 100%")}
            </span>
          </label>
          <div className="xc-dialog-actions">
            <span className="xc-spacer" />
            <button
              type="button"
              className="xc-btn primary"
              disabled={save.isPending}
              onClick={() =>
                save.mutate(
                  { defaults: value, repos: settings.data.repos, ciQuota },
                  {
                    onSuccess: () => toast(t("Saved")),
                    onError: (error) =>
                      toast({ message: errorMessage(error), tone: "error" }),
                  },
                )
              }
            >
              {t("Save")}
            </button>
          </div>
        </>
      )}
    </section>
  );
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
        <h2>{t("Watched repositories")}</h2>
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

/** B109：每月免费 CI 分钟数，仓库页的“本月 CI 时长”按它算百分比。 */
function CIQuotaCard() {
  const t = useT();
  const config = useGitHubConfig();
  const save = useSaveGitHubConfig();
  const [minutes, setMinutes] = useState("");
  const saved = config.data?.ciIncludedMinutes;
  useEffect(() => {
    if (saved != null) setMinutes(String(saved));
  }, [saved]);
  // 后端还没上线时接口不返回这个字段，不显示这张卡片。
  if (saved == null) return null;
  const n = Number(minutes);
  const valid = Number.isInteger(n) && n >= 1 && n <= 1_000_000;
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!valid) return;
    save.mutate(
      { ciIncludedMinutes: n },
      {
        onSuccess: () => toast(t("Saved")),
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };
  return (
    <form className="xc-card" onSubmit={onSubmit}>
      <div className="xc-card-head">
        <h2>{t("CI minutes quota")}</h2>
      </div>
      <p className="xc-muted">
        {t(
          "GitHub Free includes 2000 minutes a month. Change it after you upgrade your plan.",
        )}
      </p>
      <label className="xc-field">
        <span>{t("Free CI minutes per month")}</span>
        <input
          className="xc-input"
          type="number"
          inputMode="numeric"
          min={1}
          max={1000000}
          step={1}
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
        />
      </label>
      <div className="xc-dialog-actions">
        <span className="xc-spacer" />
        <button
          type="submit"
          className="xc-btn primary"
          disabled={!valid || save.isPending || n === saved}
        >
          {t("Save")}
        </button>
      </div>
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
