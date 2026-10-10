import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Copy, Download, Plus, Trash2 } from "lucide-react";
import { coreApi, coreKeys, useAgents, type AgentKind } from "../../api/core";
import { errorMessage, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import Dialog from "../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatTime, relativeTime } from "../../lib/time";
import {
  HostInfoFields,
  infoDraft,
  infoFilled,
  infoInput,
  type InfoDraft,
} from "../servers/components/HostInfo";
import { confirmAction } from "../../components/ui/ConfirmDialog";

export default function DevicesTab() {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const agents = useAgents();
  // 服务器页、电脑页的“添加”跳过来时带 ?add=server 或 ?add=desktop。
  const [params, setParams] = useSearchParams();
  const add = params.get("add");
  const [pairOpen, setPairOpen] = useState(
    add === "server" || add === "desktop",
  );
  const revoke = useMutation({
    mutationFn: (id: string) =>
      withElevation(() =>
        unwrap(
          coreApi.DELETE("/agents/{agentId}", {
            params: { path: { agentId: id } },
          }),
        ),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: coreKeys.agents }),
    onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
  });

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Devices & agents")}</h2>
        <button className="xc-btn small" onClick={() => setPairOpen(true)}>
          <Plus size={14} /> {t("Add a device")}
        </button>
      </div>
      {agents.isPending ? (
        <Loading />
      ) : agents.isError ? (
        <ErrorState error={agents.error} onRetry={() => agents.refetch()} />
      ) : agents.data.length === 0 ? (
        <EmptyState title={t("No devices paired yet")} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table">
            <thead>
              <tr>
                <th>{t("Device name")}</th>
                <th>{t("Device type")}</th>
                <th>{t("Version")}</th>
                <th>{t("Last seen")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {agents.data.map((agent) => (
                <tr key={agent.id}>
                  <td>
                    <div className="xc-row">
                      <span className={`xc-dot ${agent.online ? "ok" : ""}`} />
                      <strong>{agent.name}</strong>
                      <span className="xc-muted">{agent.hostname}</span>
                    </div>
                  </td>
                  <td>
                    {agent.kind === "server"
                      ? t("Linux server")
                      : t("Windows computer")}{" "}
                    · {agent.os}/{agent.arch}
                  </td>
                  <td className="xc-mono">{agent.version}</td>
                  <td>
                    {agent.online
                      ? t("Online")
                      : agent.lastSeenAt
                        ? relativeTime(agent.lastSeenAt, language)
                        : "—"}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <button
                      className="xc-btn small danger"
                      disabled={revoke.isPending}
                      onClick={async () =>
                        (await confirmAction({
                          title: `${t("Revoke")}“${agent.name}”？`,
                          description: t(
                            "The agent is disconnected and cannot reconnect until it is paired again.",
                          ),
                          confirmLabel: t("Revoke"),
                        })) && revoke.mutate(agent.id)
                      }
                    >
                      <Trash2 size={13} /> {t("Revoke")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <UninstallHelp />
      <PairDialog
        open={pairOpen}
        initialKind={add === "desktop" ? "desktop" : "server"}
        onClose={() => {
          setPairOpen(false);
          if (add) setParams({}, { replace: true });
        }}
      />
    </div>
  );
}

// 单行写完，直接粘贴到普通 PowerShell 里执行，不需要管理员。
const WINDOWS_UNINSTALL =
  "Stop-ScheduledTask -TaskName 'X Console Agent' -ErrorAction SilentlyContinue; " +
  "Unregister-ScheduledTask -TaskName 'X Console Agent' -Confirm:$false -ErrorAction SilentlyContinue; " +
  "Remove-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run' -Name 'X Console Agent' -ErrorAction SilentlyContinue; " +
  "Stop-Process -Name x-console-agent -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 1; " +
  'Remove-Item -Recurse -Force "$env:LOCALAPPDATA\\x-console-agent","$env:APPDATA\\x-console-agent" -ErrorAction SilentlyContinue';

/** 卸载和更新代理的说明，一直显示在设备列表下面。 */
function UninstallHelp() {
  const t = useT();
  const server = location.origin;
  return (
    <div className="devices-uninstall">
      <h3>{t("Uninstall and update")}</h3>
      <p className="devices-note">
        更新不用卸载，也不用先吊销：在“添加设备”里复制安装命令再执行一次，只会替换程序，不会重新配对。
        要彻底卸载，先在设备上执行下面的命令，再到上面的列表里吊销这台设备。先吊销再重装的话，旧的配对信息还在，装完仍然连不上。
      </p>
      <CommandBox
        label={t("Uninstall on Windows, run in PowerShell")}
        command={WINDOWS_UNINSTALL}
      />
      <CommandBox
        label={t("Uninstall on Linux, run as root")}
        command={`curl -fsSL ${server}/api/v1/agent/uninstall.sh | sudo sh`}
      />
      <p className="devices-note">Unraid 上去掉 sudo。</p>
    </div>
  );
}

/** 一条可以复制的命令。 */
function CommandBox({ label, command }: { label: string; command: string }) {
  const t = useT();
  return (
    <div className="xc-field">
      <span>{label}</span>
      <div className="devices-command">
        <code>{command}</code>
        <button
          type="button"
          className="xc-btn small"
          onClick={() =>
            navigator.clipboard
              .writeText(command)
              .then(() => toast(t("Copied")))
              .catch(() =>
                toast({ message: t("Could not copy"), tone: "error" }),
              )
          }
          aria-label={`${t("Copy")} ${label}`}
        >
          <Copy size={13} /> {t("Copy")}
        </button>
      </div>
    </div>
  );
}

/*
 * 添加设备（B30）：生成配对码后给一条安装命令。Linux 是 curl | sudo sh，
 * Windows 是 PowerShell 一条命令或者下载安装程序。弹窗开着时等设备连上来，
 * 连上后显示“打开”。
 */
function PairDialog({
  open,
  onClose,
  initialKind = "server",
}: {
  open: boolean;
  onClose: () => void;
  initialKind?: AgentKind;
}) {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const agents = useAgents();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<AgentKind>(initialKind);
  useEffect(() => {
    if (open) setKind(initialKind);
  }, [open, initialKind]);
  const [known, setKnown] = useState<string[]>([]);
  const [manual, setManual] = useState(false);
  const [result, setResult] = useState<{
    code: string;
    expiresAt: string;
  } | null>(null);
  const [error, setError] = useState("");
  // B82：添加时可以先填归属、账号密码、备注、标签，折叠在“更多信息”里。
  const [infoOpen, setInfoOpen] = useState(false);
  const [draft, setDraft] = useState<InfoDraft>(() => infoDraft());
  const create = useMutation({
    mutationFn: () =>
      withElevation(() =>
        unwrap(
          coreApi.POST("/agents/pairing-codes", {
            body: {
              name,
              kind,
              info: infoFilled(draft) ? infoInput(draft) : undefined,
            },
          }),
        ),
      ),
    onSuccess: (data) => {
      setKnown((agents.data ?? []).map((a) => a.id));
      setResult(data);
    },
    onError: (err) => setError(errorMessage(err)),
  });
  // 等待期间每 3 秒看一次有没有新设备（代理上线也会发事件，这里再兜底）。
  const qc = useQueryClient();
  useEffect(() => {
    if (!result) return;
    const timer = setInterval(
      () => qc.invalidateQueries({ queryKey: coreKeys.agents }),
      3000,
    );
    return () => clearInterval(timer);
  }, [result, qc]);
  const joined = result
    ? (agents.data ?? []).find((a) => !known.includes(a.id))
    : undefined;
  const close = () => {
    setResult(null);
    setName("");
    setError("");
    setManual(false);
    setInfoOpen(false);
    setDraft(infoDraft());
    onClose();
  };
  const server = location.origin;
  const code = result?.code ?? "";
  const q = encodeURIComponent(code);
  const linux = `curl -fsSL "${server}/api/v1/agent/install.sh?code=${q}" | sudo sh`;
  // B64：Unraid 没有 sudo，终端本来就是 root。
  const unraid = `curl -fsSL "${server}/api/v1/agent/install.sh?code=${q}" | sh`;
  const windows = `irm "${server}/api/v1/agent/install.ps1?code=${q}" | iex`;
  const manualCommand =
    kind === "server"
      ? `sudo x-console-agent pair --server ${server} --code ${code} --config /etc/x-console-agent/config.json`
      : `x-console-agent.exe pair --server ${server} --code ${code}`;
  return (
    <Dialog
      open={open}
      onClose={close}
      title={kind === "server" ? t("Add a server") : t("Add a computer")}
      description={
        result
          ? undefined
          : "生成一个配对码，在设备上执行一条命令就能接入。机器能访问面板地址就行，不用开端口，内网机器也可以。"
      }
    >
      {result ? (
        <>
          {kind === "server" ? (
            <>
              <CommandBox
                label={t("Run this on the server as root")}
                command={linux}
              />
              <CommandBox
                label={t("On Unraid, run this in its terminal")}
                command={unraid}
              />
              <p className="devices-note">
                Unraid 没有 sudo，用第二条。代理装在 U 盘上，重启 Unraid
                后自动启动。
              </p>
            </>
          ) : (
            <>
              <CommandBox
                label={t("Run this in PowerShell")}
                command={windows}
              />
              <div className="xc-field">
                <span>
                  {t("Or download the installer and double-click it")}
                </span>
                <a
                  className="xc-btn"
                  href={`/api/v1/agent/setup.exe?code=${q}`}
                  download={`x-console-agent-setup-${code}.exe`}
                >
                  <Download size={14} /> {t("Download the installer")}
                </a>
              </div>
            </>
          )}
          <p className="devices-note">
            {t("The code is")} <code>{code}</code>，
            {t("valid for one use until")}{" "}
            {formatTime(result.expiresAt, language)}。
          </p>
          <div className={`devices-wait${joined ? " ok" : ""}`} role="status">
            {joined ? (
              <>
                <span className="xc-dot ok" /> {joined.name} {t("is connected")}
              </>
            ) : (
              <>
                <span className="devices-spinner" />{" "}
                {t("Waiting for the device to connect…")}
              </>
            )}
          </div>
          <button
            type="button"
            className="devices-manual-toggle"
            aria-expanded={manual}
            onClick={() => setManual((v) => !v)}
          >
            {t("Install by hand")}
          </button>
          {manual && (
            <>
              <p className="devices-note">
                {kind === "server"
                  ? "把 x-console-agent 复制到 /usr/local/bin 后执行："
                  : "把 x-console-agent.exe 放到任意目录后执行："}
              </p>
              <CommandBox label={t("Pair")} command={manualCommand} />
              <p className="devices-note">
                {kind === "server"
                  ? "配对只保存配置。再执行 x-console-agent run --config /etc/x-console-agent/config.json 才会连上面板。"
                  : "在 Windows 上配对后，代理会自动在后台运行并设成登录时启动，关掉终端也不会停。要自己再启动一次，执行 x-console-agent.exe install。"}
              </p>
            </>
          )}
          {kind === "server" && (
            <p className="devices-note">
              {t("To uninstall:")}{" "}
              <code>
                curl -fsSL {server}/api/v1/agent/uninstall.sh | sudo sh
              </code>
              （Unraid 上去掉 sudo）
            </p>
          )}
          <div className="xc-dialog-actions">
            <button className="xc-btn" onClick={close}>
              {t("Close")}
            </button>
            {joined && (
              <button
                className="xc-btn primary"
                onClick={() => {
                  close();
                  navigate(
                    joined.kind === "server"
                      ? `/servers/${encodeURIComponent(joined.id)}`
                      : "/pc",
                  );
                }}
              >
                {t("Open it")}
              </button>
            )}
          </div>
        </>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <label className="xc-field">
            <span>{t("Device name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如 tokyo-1 或 我的台式机"
              required
              autoFocus
            />
          </label>
          <label className="xc-field">
            <span>{t("Device type")}</span>
            <select
              className="xc-select"
              value={kind}
              onChange={(e) => setKind(e.target.value as AgentKind)}
            >
              <option value="server">{t("Linux server")}</option>
              <option value="desktop">{t("Windows computer")}</option>
            </select>
          </label>
          <button
            type="button"
            className="devices-manual-toggle"
            aria-expanded={infoOpen}
            onClick={() => setInfoOpen((v) => !v)}
          >
            {t("More info")}
          </button>
          {infoOpen && <HostInfoFields draft={draft} onChange={setDraft} />}
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-dialog-actions">
            <button type="button" className="xc-btn ghost" onClick={close}>
              {t("Cancel")}
            </button>
            <button className="xc-btn primary" disabled={create.isPending}>
              {t("Generate code")}
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
