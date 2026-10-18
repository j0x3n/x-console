import { useEffect, useState } from "react";
import { Link } from "react-router";
import { KeyRound, Plus, RefreshCw, Trash2, Webhook } from "lucide-react";
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
import { useGitHubNotify, useSaveGitHubNotify, type RepoNotify } from "./api";
import { ForgeIcon } from "./RepoBits";
import { DEFAULT_NOTIFY } from "./logic";
import NotifyFields from "./NotifyFields";
import GitAccountDialog, {
  TokenDialog,
  WebhookDialog,
} from "./GitAccountDialog";
import "../aiagents/aiagents.css";
import "./i18n";
import "./github.css";

/**
 * 设置 → Git 与 GitHub（B62）。Git 账号是唯一填令牌的地方：
 * Agent 用它克隆和提 PR，仓库页用它同步。
 */
export default function GitSettingsTab() {
  // B248：这里只留账号和通知。关注哪些仓库、同步状态、CI 分钟数在仓库页的“管理仓库”里。
  const t = useT();
  return (
    <div className="settings-grid">
      <AccountsCard />
      <NotifyCard />
      <section className="xc-card">
        <div className="settings-line">
          <div>
            <strong>{t("Watched repositories")}</strong>
            <small>{t("Pick repositories on the repositories page.")}</small>
          </div>
          <Link className="xc-btn small" to="/github?manage=1">
            {t("Manage repositories")}
          </Link>
        </div>
      </section>
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
