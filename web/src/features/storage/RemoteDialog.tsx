import { useEffect, useState, type FormEvent } from "react";
import { Copy } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import {
  gdriveRedirectUri,
  useSaveRemote,
  useStartRemoteAuth,
  useTestRemote,
  type StorageRemote,
  type StorageRemoteInput,
  type StorageRemoteKind,
} from "./api";

type Form = {
  kind: StorageRemoteKind;
  name: string;
  showInDrive: boolean;
  url: string;
  username: string;
  password: string;
  clientId: string;
  clientSecret: string;
};

function formOf(r?: StorageRemote | null): Form {
  return {
    kind: r?.kind ?? "webdav",
    name: r?.name ?? "",
    showInDrive: r?.showInDrive ?? true,
    url: r?.webdav?.url ?? "",
    username: r?.webdav?.username ?? "",
    password: "",
    clientId: r?.gdrive?.clientId ?? "",
    clientSecret: "",
  };
}

/** 只发要保存的字段。密码和密钥留空表示不改。 */
export function remoteInput(f: Form, create: boolean): StorageRemoteInput {
  const out: StorageRemoteInput = {
    name: f.name.trim(),
    showInDrive: f.showInDrive,
  };
  if (create) out.kind = f.kind;
  if (f.kind === "webdav") {
    out.webdav = { url: f.url.trim(), username: f.username.trim() };
    if (f.password) out.webdav.password = f.password;
  } else {
    out.gdrive = { clientId: f.clientId.trim() };
    if (f.clientSecret.trim()) out.gdrive.clientSecret = f.clientSecret.trim();
  }
  return out;
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
  hint,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  hint?: string;
  type?: "text" | "password";
}) {
  return (
    <label className="xc-field">
      <span>{label}</span>
      <input
        className="xc-input"
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete={type === "password" ? "new-password" : "off"}
        spellCheck={false}
      />
      {hint && <small>{hint}</small>}
    </label>
  );
}

/** 添加或修改网盘账号（B69）。WebDAV 保存时服务端先连一次。 */
export default function RemoteDialog({
  open,
  onClose,
  remote,
}: {
  open: boolean;
  onClose: () => void;
  remote: StorageRemote | null;
}) {
  const t = useT();
  const save = useSaveRemote();
  const test = useTestRemote();
  const startAuth = useStartRemoteAuth();
  const [f, setF] = useState<Form>(() => formOf(remote));
  const [error, setError] = useState("");
  const [tested, setTested] = useState<{ ok: boolean; message: string }>();

  useEffect(() => {
    if (!open) return;
    setF(formOf(remote));
    setError("");
    setTested(undefined);
  }, [open, remote]);

  const set = <K extends keyof Form>(k: K, v: Form[K]) =>
    setF((cur) => ({ ...cur, [k]: v }));
  const create = !remote;
  const gdrive = f.kind === "gdrive";
  const redirect = remote?.gdrive?.redirectUri ?? gdriveRedirectUri();
  // 新的 Google 账号，或者换了客户端的，保存后直接去授权
  const needsAuth =
    gdrive &&
    (create ||
      !remote?.gdrive?.authorized ||
      f.clientId.trim() !== remote?.gdrive?.clientId);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const saved = await save.mutateAsync({
        id: remote?.id,
        body: remoteInput(f, create),
      });
      if (needsAuth) {
        const r = await startAuth.mutateAsync(saved.id);
        window.location.href = r.url;
        return;
      }
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const onTest = () => {
    setTested(undefined);
    test.mutate(
      { id: remote?.id, body: remoteInput(f, create) },
      {
        onSuccess: setTested,
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  const onCopy = async () => {
    if (await copyText(redirect)) toast(t("Copied"));
  };

  const busy = save.isPending || startAuth.isPending;
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={create ? t("Add drive account") : t("Edit drive account")}
    >
      <form className="storage-remote-form" onSubmit={submit}>
        {create && (
          <label className="xc-field">
            <span>{t("Type")}</span>
            <select
              className="xc-select"
              value={f.kind}
              onChange={(e) => set("kind", e.target.value as StorageRemoteKind)}
            >
              <option value="webdav">
                WebDAV（坚果云、Nextcloud、群晖、alist 等）
              </option>
              <option value="gdrive">Google Drive</option>
            </select>
          </label>
        )}
        <TextField
          label={t("Name")}
          value={f.name}
          onChange={(v) => set("name", v)}
          placeholder={gdrive ? "Google Drive" : t("Empty means the host name")}
        />
        {!gdrive ? (
          <>
            <TextField
              label={t("Address")}
              value={f.url}
              onChange={(v) => set("url", v)}
              placeholder="https://dav.jianguoyun.com/dav/"
              hint="坚果云填 https://dav.jianguoyun.com/dav/。"
            />
            <TextField
              label={t("Username")}
              value={f.username}
              onChange={(v) => set("username", v)}
            />
            <TextField
              label={t("Password")}
              type="password"
              value={f.password}
              onChange={(v) => set("password", v)}
              placeholder={
                remote?.webdav?.passwordSet ? "已保存，不改就留空" : ""
              }
              hint="坚果云要用应用密码，在坚果云网页版的“安全选项”里生成。"
            />
          </>
        ) : (
          <>
            <ol className="storage-steps">
              <li>在 Google Cloud 控制台建一个项目，启用 Google Drive API。</li>
              <li>配置 OAuth 同意屏幕：用户类型选“外部”，再点“发布应用”。</li>
              <li>
                建一个“Web 应用”类型的 OAuth 客户端，已获授权的重定向 URI 填：
                <span className="storage-redirect">
                  <code>{redirect}</code>
                  <button
                    type="button"
                    className="xc-btn ghost small"
                    aria-label={t("Copy")}
                    title={t("Copy")}
                    onClick={onCopy}
                  >
                    <Copy size={13} />
                  </button>
                </span>
              </li>
              <li>
                把客户端 ID 和密钥填到下面，保存后在 Google 页面同意授权。
              </li>
            </ol>
            <TextField
              label={t("Client ID")}
              value={f.clientId}
              onChange={(v) => set("clientId", v)}
              placeholder="xxxx.apps.googleusercontent.com"
            />
            <TextField
              label={t("Client secret")}
              type="password"
              value={f.clientSecret}
              onChange={(v) => set("clientSecret", v)}
              placeholder={
                remote?.gdrive?.secretSet ? "已保存，不改就留空" : ""
              }
              hint="同意屏幕要发布成正式版，不然 7 天后授权会过期。没经过 Google 审核也能发布。"
            />
          </>
        )}
        <label className="xc-check">
          <input
            type="checkbox"
            checked={f.showInDrive}
            onChange={(e) => set("showInDrive", e.target.checked)}
          />
          <span>{t("Show on the drive page")}</span>
        </label>
        {tested && (
          <p
            className={`storage-test ${tested.ok ? "ok" : "failed"}`}
            role="status"
          >
            {tested.message}
          </p>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          {!gdrive && (
            <button
              type="button"
              className="xc-btn"
              disabled={test.isPending}
              onClick={onTest}
            >
              {test.isPending ? t("Testing") : t("Test connection")}
            </button>
          )}
          <button className="xc-btn primary" disabled={busy}>
            {needsAuth ? t("Save and authorize") : t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
