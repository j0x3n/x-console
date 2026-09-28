import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { errorMessage, unwrap } from "../../../api/client";
import { withElevation } from "../../../auth/elevation";
import Dialog from "../../../components/ui/Dialog";
import { EmptyState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  hostsApi,
  hostsKeys,
  useSshHosts,
  type SshHost,
  type SshHostInput,
} from "../api";

/** SSH 主机管理：列表、添加、修改、删除、测试连接。 */
export default function SshHostsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const list = useSshHosts();
  const [editing, setEditing] = useState<SshHost | "new" | null>(null);
  const remove = async (h: SshHost) => {
    if (!confirm(`${t("Delete")} ${h.name}?`)) return;
    try {
      await withElevation(() =>
        unwrap(
          hostsApi.DELETE("/ssh-hosts/{sshId}", {
            params: { path: { sshId: h.id } },
          }),
        ),
      );
      qc.invalidateQueries({ queryKey: hostsKeys.ssh });
      qc.invalidateQueries({ queryKey: hostsKeys.lists });
      toast(t("Deleted"));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };
  if (editing)
    return (
      <SshHostForm
        host={editing === "new" ? null : editing}
        onClose={() => setEditing(null)}
      />
    );
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("SSH hosts")}
      description={t(
        "For servers without the agent. They support terminal, commands and basic metrics.",
      )}
      wide
    >
      {list.isPending ? (
        <Loading />
      ) : (list.data ?? []).length === 0 ? (
        <EmptyState title={t("No SSH hosts yet")} />
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table servers-table">
            <thead>
              <tr>
                <th>{t("Name")}</th>
                <th>{t("Address")}</th>
                <th>{t("Host key")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {list.data!.map((h) => (
                <tr key={h.id}>
                  <td>
                    <strong>{h.name}</strong>
                  </td>
                  <td className="xc-mono">
                    {h.username}@{h.address}:{h.port}
                  </td>
                  <td className="xc-mono xc-muted servers-fp">
                    {h.hostKeyFingerprint || t("Learned on first connect")}
                  </td>
                  <td className="servers-actions">
                    <button
                      className="xc-btn small ghost"
                      onClick={() => setEditing(h)}
                      aria-label={t("Edit")}
                    >
                      <Pencil size={13} />
                    </button>
                    <button
                      className="xc-btn small ghost"
                      onClick={() => remove(h)}
                      aria-label={t("Delete")}
                    >
                      <Trash2 size={13} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="xc-dialog-actions">
        <button className="xc-btn" onClick={() => setEditing("new")}>
          <Plus size={14} /> {t("Add SSH host")}
        </button>
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

function SshHostForm({
  host,
  onClose,
}: {
  host: SshHost | null;
  onClose: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const [form, setForm] = useState<SshHostInput>({
    name: host?.name ?? "",
    address: host?.address ?? "",
    port: host?.port ?? 22,
    username: host?.username ?? "root",
    auth: host?.auth ?? "password",
    secret: "",
    passphrase: "",
  });
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(
    null,
  );
  const [busy, setBusy] = useState(false);
  const set = (patch: Partial<SshHostInput>) =>
    setForm((f) => ({ ...f, ...patch }));
  const body = () => ({
    ...form,
    secret: form.secret || undefined,
    passphrase: form.passphrase || undefined,
  });
  const test = async () => {
    setBusy(true);
    setResult(null);
    try {
      const res =
        host && !form.secret
          ? await unwrap(
              hostsApi.POST("/ssh-hosts/{sshId}/test", {
                params: { path: { sshId: host.id } },
              }),
            )
          : await withElevation(() =>
              unwrap(hostsApi.POST("/ssh-hosts/test", { body: body() })),
            );
      setResult({
        ok: res.ok,
        message: res.fingerprint
          ? `${res.message} · ${res.fingerprint}`
          : res.message,
      });
    } catch (e) {
      setResult({ ok: false, message: errorMessage(e) });
    } finally {
      setBusy(false);
    }
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await withElevation(() =>
        host
          ? unwrap(
              hostsApi.PUT("/ssh-hosts/{sshId}", {
                params: { path: { sshId: host.id } },
                body: body(),
              }),
            )
          : unwrap(hostsApi.POST("/ssh-hosts", { body: body() })),
      );
      qc.invalidateQueries({ queryKey: hostsKeys.ssh });
      qc.invalidateQueries({ queryKey: hostsKeys.lists });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setResult({ ok: false, message: errorMessage(err) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={t(host ? "Edit SSH host" : "Add SSH host")}
      description={t("The password or private key is stored encrypted.")}
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={form.name}
            onChange={(e) => set({ name: e.target.value })}
            placeholder="old-box"
            required
            autoFocus
          />
        </label>
        <div className="servers-form-row">
          <label className="xc-field servers-grow">
            <span>{t("Address")}</span>
            <input
              className="xc-input"
              value={form.address}
              onChange={(e) => set({ address: e.target.value })}
              placeholder="203.0.113.5"
              required
            />
          </label>
          <label className="xc-field servers-port">
            <span>{t("Port")}</span>
            <input
              className="xc-input"
              type="number"
              min={1}
              max={65535}
              value={form.port}
              onChange={(e) => set({ port: Number(e.target.value) })}
            />
          </label>
        </div>
        <div className="servers-form-row">
          <label className="xc-field">
            <span>{t("Username")}</span>
            <input
              className="xc-input"
              value={form.username}
              onChange={(e) => set({ username: e.target.value })}
              required
            />
          </label>
          <label className="xc-field">
            <span>{t("Sign in with")}</span>
            <select
              className="xc-select"
              value={form.auth}
              onChange={(e) =>
                set({ auth: e.target.value as SshHostInput["auth"] })
              }
            >
              <option value="password">{t("Password")}</option>
              <option value="key">{t("Private key")}</option>
            </select>
          </label>
        </div>
        {form.auth === "password" ? (
          <label className="xc-field">
            <span>{t("Password")}</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="new-password"
              value={form.secret}
              onChange={(e) => set({ secret: e.target.value })}
              placeholder={host ? t("Leave empty to keep") : ""}
              required={!host}
            />
          </label>
        ) : (
          <>
            <label className="xc-field">
              <span>{t("Private key")}</span>
              <textarea
                className="xc-textarea xc-mono"
                value={form.secret}
                onChange={(e) => set({ secret: e.target.value })}
                placeholder={
                  host
                    ? t("Leave empty to keep")
                    : "-----BEGIN OPENSSH PRIVATE KEY-----"
                }
                required={!host}
              />
            </label>
            <label className="xc-field">
              <span>{t("Key passphrase (optional)")}</span>
              <input
                className="xc-input"
                type="password"
                autoComplete="off"
                value={form.passphrase}
                onChange={(e) => set({ passphrase: e.target.value })}
              />
            </label>
          </>
        )}
        {result && (
          <p className={result.ok ? "servers-ok-text" : "xc-error-text"}>
            {result.message}
          </p>
        )}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="button"
            className="xc-btn"
            onClick={test}
            disabled={busy || !form.address}
          >
            {t("Test connection")}
          </button>
          <button className="xc-btn primary" disabled={busy}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
