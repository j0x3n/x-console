import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { coreApi, coreKeys, useAgents, type AgentKind } from "../../api/core";
import { errorMessage, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import Dialog from "../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { confirmAction } from "../../components/ui/ConfirmDialog";

export default function DevicesTab() {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const agents = useAgents();
  const [pairOpen, setPairOpen] = useState(false);
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
          <Plus size={14} /> {t("Pair a new device")}
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
                      : t("Windows PC")}{" "}
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
      <PairDialog open={pairOpen} onClose={() => setPairOpen(false)} />
    </div>
  );
}

function PairDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<AgentKind>("server");
  const [result, setResult] = useState<{
    code: string;
    expiresAt: string;
  } | null>(null);
  const [error, setError] = useState("");
  const create = useMutation({
    mutationFn: () =>
      withElevation(() =>
        unwrap(coreApi.POST("/agents/pairing-codes", { body: { name, kind } })),
      ),
    onSuccess: (data) => setResult(data),
    onError: (err) => setError(errorMessage(err)),
  });
  const close = () => {
    setResult(null);
    setName("");
    setError("");
    onClose();
  };
  const server = location.origin;
  const command =
    kind === "server"
      ? `sudo x-console-agent pair --server ${server} --code ${result?.code} --config /etc/x-console-agent/config.json`
      : `x-console-agent.exe pair --server ${server} --code ${result?.code}`;
  return (
    <Dialog
      open={open}
      onClose={close}
      title={t("Pair a new device")}
      description="在要管理的机器上安装 x-console-agent，然后用配对码完成绑定。配对码 10 分钟内有效，只能用一次。"
    >
      {result ? (
        <>
          <label className="xc-field">
            <span>{t("Pairing code")}</span>
            <code
              className="xc-secret"
              style={{ fontSize: 20, textAlign: "center" }}
            >
              {result.code}
            </code>
          </label>
          <label className="xc-field">
            <span>在设备上运行</span>
            <code className="xc-secret">{command}</code>
          </label>
          <div className="xc-dialog-actions">
            <button className="xc-btn primary" onClick={close}>
              {t("Close")}
            </button>
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
              <option value="desktop">{t("Windows PC")}</option>
            </select>
          </label>
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
