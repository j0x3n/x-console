import { useEffect, useState, type FormEvent } from "react";
import { PlugZap } from "lucide-react";
import { useAgents } from "../../api/core";
import { errorMessage } from "../../api/client";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useHAConfig,
  useHAStatus,
  useSaveConfig,
  useTestHA,
  type HAConfig,
  type HAConfigInput,
  type HAMode,
  type HATestResult,
} from "./api";
import "./i18n";
import "./home.css";

/** 代理能转发网络请求时上报的能力。 */
const CAP_PROXY = "proxy";

export default function HASettingsTab() {
  const config = useHAConfig();
  if (config.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  return (
    <div className="home-settings">
      <ConfigForm initial={config.data} />
      <StatusCard />
    </div>
  );
}

function ConfigForm({ initial }: { initial: HAConfig }) {
  const t = useT();
  const agents = useAgents();
  const save = useSaveConfig();
  const test = useTestHA();
  const [url, setUrl] = useState(initial.url);
  const [token, setToken] = useState("");
  const [mode, setMode] = useState<HAMode>(initial.mode);
  const [agentId, setAgentId] = useState(initial.agentId);
  const [result, setResult] = useState<HATestResult | null>(null);

  useEffect(() => {
    setUrl(initial.url);
    setMode(initial.mode);
    setAgentId(initial.agentId);
  }, [initial]);

  const input = (): HAConfigInput => ({
    url: url.trim(),
    token: token.trim() || undefined,
    mode,
    agentId: mode === "agent" ? agentId : undefined,
  });

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(input(), {
      onSuccess: () => {
        setToken("");
        toast(t("Saved"));
      },
      onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
    });
  };

  const onTest = () => {
    setResult(null);
    test.mutate(input(), {
      onSuccess: setResult,
      onError: (error) =>
        setResult({ ok: false, message: errorMessage(error) }),
    });
  };

  const proxyAgents = (agents.data ?? []).filter((a) =>
    a.capabilities.includes(CAP_PROXY),
  );

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>Home Assistant</h2>
      </div>
      <label className="xc-field">
        <span>{t("Address")}</span>
        <input
          className="xc-input"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="http://homeassistant.local:8123"
          inputMode="url"
          autoComplete="off"
        />
        <small>
          HA 有公网地址时填公网地址，比如 Nabu Casa 或自己的反向代理。只在家里能访问时，选“通过代理”，填家里的地址。
        </small>
      </label>
      <label className="xc-field">
        <span>{t("Long-lived access token")}</span>
        <input
          className="xc-input"
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={
            initial.hasToken
              ? `${initial.token}（${t("leave empty to keep")}）`
              : ""
          }
          autoComplete="new-password"
        />
        <small>在 HA 左下角点头像，打开“安全”页，在最下面创建长期访问令牌。</small>
      </label>
      <label className="xc-field">
        <span>{t("Connection mode")}</span>
        <select
          className="xc-select"
          value={mode}
          onChange={(e) => setMode(e.target.value as HAMode)}
        >
          <option value="direct">{t("Direct from the server")}</option>
          <option value="agent">{t("Through an agent at home")}</option>
        </select>
      </label>
      {mode === "agent" && (
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
          </select>
          <small>
            选一台和 HA 在同一个网络里的设备。代理只转发到内网地址。
            {proxyAgents.length === 0 && " 还没有支持转发的代理，请先在“设备与代理”里配对，或升级代理程序。"}
          </small>
        </label>
      )}
      {result && (
        <p
          className={`home-test-result ${result.ok ? "ok" : "fail"}`}
          role="status"
        >
          {result.ok
            ? `${t("Connection works")}：${result.locationName ?? ""} · Home Assistant ${result.version ?? ""}`
            : result.message}
        </p>
      )}
      <div className="xc-dialog-actions">
        <button
          type="button"
          className="xc-btn"
          disabled={test.isPending || !url.trim()}
          onClick={onTest}
        >
          <PlugZap size={14} /> {test.isPending ? t("Testing") : t("Test connection")}
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
  const status = useHAStatus();
  if (!status.data) return null;
  const s = status.data;
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Status")}</h2>
      </div>
      {!s.configured ? (
        <p className="xc-muted">{t("Not configured")}</p>
      ) : (
        <dl className="home-status">
          <dt>{t("Connection status")}</dt>
          <dd>
            {s.connected ? (
              <span className="xc-badge ok">{t("Connected")}</span>
            ) : (
              <span className="xc-badge warn">{t("Not connected")}</span>
            )}
          </dd>
          <dt>{t("Connection mode")}</dt>
          <dd>
            {s.mode === "agent"
              ? t("Through an agent at home")
              : t("Direct from the server")}
          </dd>
          {s.version && (
            <>
              <dt>{t("Version")}</dt>
              <dd className="xc-mono">{s.version}</dd>
            </>
          )}
          <dt>{t("Entities")}</dt>
          <dd>{s.entityCount}</dd>
          {s.connectedAt && (
            <>
              <dt>{t("Connected since")}</dt>
              <dd>{relativeTime(s.connectedAt, language)}</dd>
            </>
          )}
          {s.error && (
            <>
              <dt>{t("Last error")}</dt>
              <dd className="xc-error-text">{s.error}</dd>
            </>
          )}
        </dl>
      )}
    </div>
  );
}
