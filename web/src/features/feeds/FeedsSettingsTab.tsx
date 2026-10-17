import { useEffect, useState, type FormEvent } from "react";
import { PlugZap } from "lucide-react";
import { useAgents } from "../../api/core";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useFeedsSettings,
  useSaveFeedsSettings,
  useTestFeeds,
  type FeedsHealth,
  type FeedsSettings,
} from "./api";
import "./i18n";
import "./feeds.css";

/** 代理能转发网络请求时上报的能力。 */
const CAP_PROXY = "proxy";

/** 设置 → 资讯（B241）：选虚拟机、接口地址和令牌。 */
export default function FeedsSettingsTab() {
  const settings = useFeedsSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  return (
    <div className="settings-grid">
      <SettingsForm initial={settings.data} />
    </div>
  );
}

function SettingsForm({ initial }: { initial: FeedsSettings }) {
  const t = useT();
  const language = useLanguage();
  const agents = useAgents();
  const save = useSaveFeedsSettings();
  const test = useTestFeeds();
  const [agentId, setAgentId] = useState(initial.agentId);
  const [baseUrl, setBaseUrl] = useState(initial.baseUrl);
  const [token, setToken] = useState("");
  const [health, setHealth] = useState<FeedsHealth | null>(null);
  const [testError, setTestError] = useState("");

  useEffect(() => {
    setAgentId(initial.agentId);
    setBaseUrl(initial.baseUrl);
  }, [initial]);

  const proxyAgents = (agents.data ?? []).filter((a) =>
    a.capabilities.includes(CAP_PROXY),
  );
  const dirty =
    agentId !== initial.agentId ||
    baseUrl.trim() !== initial.baseUrl ||
    token.trim() !== "";

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        agentId,
        baseUrl: baseUrl.trim() || undefined,
        token: token.trim() || undefined,
      },
      {
        onSuccess: () => {
          setToken("");
          toast(t("Saved"));
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const onTest = () => {
    setHealth(null);
    setTestError("");
    test.mutate(undefined, {
      onSuccess: setHealth,
      onError: (error) => setTestError(errorMessage(error)),
    });
  };

  const onTurnOff = async () => {
    const ok = await confirmAction({
      title: t("Turn off feeds?"),
      description: t(
        "The machine, address and token are cleared. The data on the machine is not touched.",
      ),
      confirmLabel: t("Turn off"),
    });
    if (!ok) return;
    save.mutate(
      { agentId: "" },
      {
        onSuccess: () => {
          setToken("");
          setHealth(null);
          toast(t("Turned off"));
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("Feeds service")}</h2>
        {initial.configured ? (
          <span className="xc-badge ok">{t("Configured")}</span>
        ) : (
          <span className="xc-badge">{t("Not configured")}</span>
        )}
      </div>
      <div className="settings-form">
        <label className="xc-field">
          <span>{t("Agent")}</span>
          <select
            className="xc-select"
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
          >
            <option value="">{t("Choose an agent")}</option>
            {proxyAgents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
                {a.online ? "" : ` (${t("Offline")})`}
              </option>
            ))}
            {agentId &&
              !proxyAgents.some((a) => a.id === agentId) &&
              !agents.isPending && <option value={agentId}>{agentId}</option>}
          </select>
          <small>
            选跑着 ai-news 和 x-assistant
            的那台虚拟机。面板经过它的代理读数据，数据不存进面板。
            {proxyAgents.length === 0 &&
              !agents.isPending &&
              " 还没有支持转发的代理，请先在“设备与代理”里配对。"}
          </small>
        </label>
        <label className="xc-field">
          <span>{t("Service address")}</span>
          <input
            className="xc-input"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            placeholder="http://127.0.0.1:8790"
            inputMode="url"
            autoComplete="off"
          />
          <small>小接口只监听虚拟机本机，一般不用改。</small>
        </label>
        <label className="xc-field">
          <span>{t("Service token")}</span>
          <input
            className="xc-input"
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={
              initial.hasToken ? `••••••（${t("leave empty to keep")}）` : ""
            }
            autoComplete="new-password"
          />
          <small>
            和虚拟机上 /etc/x-console-feeds.env 里的 XC_FEEDS_TOKEN 一样。
          </small>
        </label>
      </div>
      <details className="settings-help">
        <summary>{t("How to install the service")}</summary>
        <ol>
          <li>在虚拟机上装好面板代理，并在“设备与代理”里配对。</li>
          <li>
            把仓库里 deploy/feeds-api 目录下的 feeds_api.py 放到
            /opt/x-console-feeds/。
          </li>
          <li>
            生成一个令牌，写进
            /etc/x-console-feeds.env：XC_FEEDS_TOKEN=令牌，权限设成 0600。
          </li>
          <li>
            装 x-console-feeds.service，然后 systemctl enable --now
            x-console-feeds。
          </li>
          <li>给 x-assistant 打上 collect.js.patch，推文才会存下来。</li>
        </ol>
        <p>详细步骤见 deploy/feeds-api/README.md。</p>
      </details>
      {health && (
        <p className="feeds-test-result ok" role="status">
          {t("News")} {health.news.count}
          {health.news.lastFetchedAt &&
            ` · ${relativeTime(health.news.lastFetchedAt, language)}`}
          {"，"}
          {t("Tweets")} {health.tweets.count}
          {health.tweets.lastAt &&
            ` · ${relativeTime(health.tweets.lastAt, language)}`}
        </p>
      )}
      {testError && (
        <p className="feeds-test-result fail" role="alert">
          {testError}
        </p>
      )}
      <div className="settings-actions">
        {initial.configured && (
          <button
            type="button"
            className="xc-btn ghost danger"
            disabled={save.isPending}
            onClick={() => void onTurnOff()}
          >
            {t("Turn off")}
          </button>
        )}
        <button
          type="button"
          className="xc-btn"
          disabled={test.isPending || !initial.configured || dirty}
          title={dirty ? "先保存再测试" : undefined}
          onClick={onTest}
        >
          <PlugZap size={14} />{" "}
          {test.isPending ? t("Testing") : t("Test connection")}
        </button>
        <button
          className="xc-btn primary"
          disabled={
            save.isPending || !agentId || (!initial.hasToken && !token.trim())
          }
        >
          {t("Save")}
        </button>
      </div>
    </form>
  );
}
