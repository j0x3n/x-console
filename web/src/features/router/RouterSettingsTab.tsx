import { useEffect, useState, type FormEvent } from "react";
import { useAgents } from "../../api/core";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import {
  useRouterConfig,
  useSaveRouterConfig,
  type RouterConfig,
  type RouterMode,
} from "./api";
import "./i18n";
import "./router.css";

/** 代理能转发网络请求时上报的能力。 */
const CAP_PROXY = "proxy";

/** 设置 → 路由器（B65）。 */
export default function RouterSettingsTab() {
  const config = useRouterConfig();
  if (config.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  return (
    <div className="settings-grid">
      <ConfigForm initial={config.data} />
      <PrepareCard />
    </div>
  );
}

function ConfigForm({ initial }: { initial: RouterConfig }) {
  const t = useT();
  const agents = useAgents();
  const save = useSaveRouterConfig();
  const [url, setUrl] = useState(initial.url);
  const [username, setUsername] = useState(initial.username || "xconsole");
  const [password, setPassword] = useState("");
  const [mode, setMode] = useState<RouterMode>(initial.mode);
  const [agentId, setAgentId] = useState(initial.agentId ?? "");

  useEffect(() => {
    setUrl(initial.url);
    setUsername(initial.username || "xconsole");
    setMode(initial.mode);
    setAgentId(initial.agentId ?? "");
  }, [initial]);

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        url: url.trim(),
        username: username.trim(),
        password: password || undefined,
        mode,
        agentId: mode === "agent" ? agentId : undefined,
      },
      {
        onSuccess: () => {
          setPassword("");
          toast(t("Saved. The router answered."));
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const onRemove = async () => {
    const ok = await confirmAction({
      title: t("Remove the router settings?"),
      description: "路由器页会回到没有设置的样子，已经采集的流量记录会留着。",
      confirmLabel: t("Remove router"),
    });
    if (!ok) return;
    save.mutate(
      { url: "" },
      {
        onSuccess: () => toast(t("Router removed")),
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const proxyAgents = (agents.data ?? []).filter((a) =>
    a.capabilities.includes(CAP_PROXY),
  );

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("Router")}</h2>
      </div>
      <label className="xc-field">
        <span>{t("Address")}</span>
        <input
          className="xc-input"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="http://192.168.1.1"
          inputMode="url"
          autoComplete="off"
        />
        <small>
          路由器的管理地址。面板服务器访问不到家里的地址时，连接方式选“通过家里的代理”。
        </small>
      </label>
      <div className="router-form-row">
        <label className="xc-field">
          <span>{t("Username")}</span>
          <input
            className="xc-input"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="off"
          />
        </label>
        <label className="xc-field">
          <span>{t("Password")}</span>
          <input
            className="xc-input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={initial.hasPassword ? t("leave empty to keep") : ""}
            autoComplete="new-password"
          />
        </label>
      </div>
      <label className="xc-field">
        <span>{t("Connection mode")}</span>
        <select
          className="xc-select"
          value={mode}
          onChange={(e) => setMode(e.target.value as RouterMode)}
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
            选一台在家里网络里、开着机的设备。这台设备掉线时，路由器页也看不到数据。
            {proxyAgents.length === 0 &&
              " 还没有支持转发的代理，请先在“设备与代理”里配对。"}
          </small>
        </label>
      )}
      <div className="xc-dialog-actions">
        {initial.url && (
          <button
            type="button"
            className="xc-btn ghost"
            disabled={save.isPending}
            onClick={onRemove}
          >
            {t("Remove router")}
          </button>
        )}
        <button
          className="xc-btn primary"
          disabled={save.isPending || !url.trim()}
        >
          {t("Save and check")}
        </button>
      </div>
    </form>
  );
}

// 路由器上的 rpcd 权限：只读状态，外加重启接口和重启路由器。
const ACL = `cat > /usr/share/rpcd/acl.d/x-console.json <<'EOF'
{
  "x-console": {
    "description": "X Console",
    "read": {
      "ubus": {
        "system": ["board", "info"],
        "network.interface": ["dump"],
        "network.device": ["status"],
        "luci-rpc": ["getHostHints", "getDHCPLeases"]
      }
    },
    "write": {
      "ubus": {
        "network.interface.*": ["up", "down"],
        "system": ["reboot"]
      }
    }
  }
}
EOF`;

const LOGIN = `uci add rpcd login
uci set rpcd.@login[-1].username='xconsole'
uci set rpcd.@login[-1].password="$(uhttpd -m '换成你的密码')"
uci add_list rpcd.@login[-1].read='x-console'
uci add_list rpcd.@login[-1].write='x-console'
uci commit rpcd
/etc/init.d/rpcd restart`;

function PrepareCard() {
  const t = useT();
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("On the router")}</h2>
      </div>
      <ol className="router-steps">
        <li>
          装好 ubus 的网页接口。装了 LuCI 的一般已经有，没有就在路由器上执行
          <code>opkg update && opkg install uhttpd-mod-ubus</code>。
        </li>
        <li>
          给面板建一个权限文件，只允许读状态、重启接口和重启路由器：
          <CodeBlock text={ACL} />
        </li>
        <li>
          建一个给面板用的账号 xconsole，密码自己定，然后把同一个密码填到左边：
          <CodeBlock text={LOGIN} />
          不建议直接用 root，root 能做任何事。
        </li>
        <li>
          面板服务器要能访问到路由器。面板在外网时，可以在家里的电脑上装代理，连接方式选“通过家里的代理”。
        </li>
      </ol>
    </section>
  );
}

function CodeBlock({ text }: { text: string }) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  return (
    <div className="router-code">
      <button
        type="button"
        className="xc-btn small"
        onClick={async () => {
          if (await copyText(text)) {
            setCopied(true);
            window.setTimeout(() => setCopied(false), 2000);
          }
        }}
      >
        {copied ? t("Copied") : t("Copy")}
      </button>
      <pre>{text}</pre>
    </div>
  );
}
