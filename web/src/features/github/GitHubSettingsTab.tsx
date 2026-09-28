import { useEffect, useState, type FormEvent } from "react";
import { PlugZap, RefreshCw } from "lucide-react";
import { errorMessage } from "../../api/client";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useGitHubConfig,
  useGitHubStatus,
  useSaveGitHubConfig,
  useSyncGitHub,
  useTestGitHub,
  type GitHubConfig,
  type GitHubTestResult,
} from "./api";
import { parseRepos } from "./logic";
import "./i18n";
import "./github.css";
import { confirmAction } from "../../components/ui/ConfirmDialog";

const DEFAULT_URL = "https://api.github.com";

export default function GitHubSettingsTab() {
  const config = useGitHubConfig();
  if (config.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  return (
    <div className="github-settings">
      <ConfigForm initial={config.data} />
      <StatusCard />
    </div>
  );
}

function ConfigForm({ initial }: { initial: GitHubConfig }) {
  const t = useT();
  const save = useSaveGitHubConfig();
  const test = useTestGitHub();
  const sync = useSyncGitHub();
  const [token, setToken] = useState("");
  const [repos, setRepos] = useState(initial.repos.join("\n"));
  const [apiUrl, setApiUrl] = useState(
    initial.apiUrl === DEFAULT_URL ? "" : initial.apiUrl,
  );
  const [result, setResult] = useState<GitHubTestResult | null>(null);

  useEffect(() => {
    setRepos(initial.repos.join("\n"));
    setApiUrl(initial.apiUrl === DEFAULT_URL ? "" : initial.apiUrl);
  }, [initial]);

  const urlChanged = (apiUrl.trim() || DEFAULT_URL) !== initial.apiUrl;

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        token: token.trim() || undefined,
        repos: parseRepos(repos),
        apiUrl: urlChanged ? apiUrl.trim() : undefined,
      },
      {
        onSuccess: (cfg) => {
          setToken("");
          toast(t("Saved"));
          // 保存后马上同步一次，页面上很快就有数据。
          if (cfg.hasToken) sync.mutate();
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const onTest = () => {
    setResult(null);
    const body =
      token.trim() || urlChanged
        ? {
            token: token.trim() || undefined,
            apiUrl: apiUrl.trim() || undefined,
          }
        : null;
    test.mutate(body, {
      onSuccess: setResult,
      onError: (error) =>
        setResult({ ok: false, message: errorMessage(error) }),
    });
  };

  const onClear = async () => {
    if (
      !(await confirmAction({
        title: t("Remove the GitHub token?"),
        description: t("Sync stops until you add a token again."),
        confirmLabel: t("Remove token"),
      }))
    )
      return;
    save.mutate(
      { repos: parseRepos(repos), clearToken: true },
      {
        onSuccess: () => toast(t("Token removed")),
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>GitHub</h2>
      </div>
      <label className="xc-field">
        <span>{t("Personal access token")}</span>
        <input
          className="xc-input"
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={
            initial.hasToken
              ? `${initial.token}（${t("leave empty to keep")}）`
              : "github_pat_..."
          }
          autoComplete="new-password"
        />
        <small>
          用细粒度令牌，在 GitHub 的 Settings → Developer settings
          里创建。仓库只选要关注的。 权限选只读：Pull
          requests、Issues、Actions、Commit statuses。要建 PR，Pull requests
          改成读写。
        </small>
      </label>
      <label className="xc-field">
        <span>{t("Watched repositories")}</span>
        <textarea
          className="xc-textarea xc-mono"
          rows={4}
          value={repos}
          onChange={(e) => setRepos(e.target.value)}
          placeholder={"owner/name\nowner/another"}
          spellCheck={false}
        />
        <small>一行一个，写成 owner/name。每 5 分钟同步一次。</small>
      </label>
      <label className="xc-field">
        <span>{t("API address")}</span>
        <input
          className="xc-input"
          value={apiUrl}
          onChange={(e) => setApiUrl(e.target.value)}
          placeholder={DEFAULT_URL}
          inputMode="url"
          autoComplete="off"
        />
        <small>一般留空。用 GitHub Enterprise 时填它的 API 地址。</small>
      </label>
      {result && (
        <p
          className={`github-test-result ${result.ok ? "ok" : "fail"}`}
          role="status"
        >
          {result.ok
            ? `${t("Token works")}：${result.login ?? ""}${result.rateLimitRemaining != null ? ` · ${t("requests left")} ${result.rateLimitRemaining}` : ""}`
            : result.message}
        </p>
      )}
      <div className="xc-dialog-actions github-actions">
        {initial.hasToken && (
          <button
            type="button"
            className="xc-btn ghost danger"
            onClick={onClear}
            disabled={save.isPending}
          >
            {t("Remove token")}
          </button>
        )}
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn"
          disabled={test.isPending || (!initial.hasToken && !token.trim())}
          onClick={onTest}
        >
          <PlugZap size={14} />{" "}
          {test.isPending ? t("Testing") : t("Test token")}
        </button>
        <button className="xc-btn primary" disabled={save.isPending}>
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
