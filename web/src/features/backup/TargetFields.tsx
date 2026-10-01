import { Copy } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import {
  gdriveRedirectUri,
  useRevokeGdriveAuth,
  type BackupSettings,
  type BackupSettingsInput,
} from "./api";
import "./i18n";

export type WebdavForm = {
  url: string;
  username: string;
  password: string;
  folder: string;
};

export type GdriveForm = {
  clientId: string;
  clientSecret: string;
  folderName: string;
};

export function webdavForm(s?: BackupSettings["webdav"]): WebdavForm {
  return {
    url: s?.url ?? "",
    username: s?.username ?? "",
    password: "",
    folder: s?.folder ?? "x-console-backups",
  };
}

export function gdriveForm(s?: BackupSettings["gdrive"]): GdriveForm {
  return {
    clientId: s?.clientId ?? "",
    clientSecret: "",
    folderName: s?.folderName ?? "X Console 备份",
  };
}

/** 只把要保存的字段发出去。密码留空表示不改。 */
export function webdavInput(f: WebdavForm): BackupSettingsInput["webdav"] {
  const out: BackupSettingsInput["webdav"] = {
    url: f.url.trim(),
    username: f.username.trim(),
    folder: f.folder.trim().replace(/^\/+|\/+$/g, ""),
  };
  if (f.password) out.password = f.password;
  return out;
}

export function gdriveInput(f: GdriveForm): BackupSettingsInput["gdrive"] {
  const out: BackupSettingsInput["gdrive"] = {
    clientId: f.clientId.trim(),
    folderName: f.folderName.trim(),
  };
  if (f.clientSecret.trim()) out.clientSecret = f.clientSecret.trim();
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

/** WebDAV 的几项输入：坚果云、Nextcloud、群晖、alist 都能用。 */
export function WebdavFields({
  value,
  onChange,
  passwordSet,
}: {
  value: WebdavForm;
  onChange: (next: WebdavForm) => void;
  passwordSet?: boolean;
}) {
  const t = useT();
  const set = <K extends keyof WebdavForm>(k: K, v: WebdavForm[K]) =>
    onChange({ ...value, [k]: v });
  return (
    <>
      <TextField
        label={t("Address")}
        value={value.url}
        onChange={(v) => set("url", v)}
        placeholder="https://dav.jianguoyun.com/dav/"
        hint="坚果云填 https://dav.jianguoyun.com/dav/。"
      />
      <TextField
        label={t("Username")}
        value={value.username}
        onChange={(v) => set("username", v)}
      />
      <TextField
        label={t("Password")}
        type="password"
        value={value.password}
        onChange={(v) => set("password", v)}
        placeholder={passwordSet ? "已保存，不改就留空" : ""}
        hint="坚果云要用应用密码，在坚果云网页版的“安全选项”里生成。"
      />
      <TextField
        label={t("Folder")}
        value={value.folder}
        onChange={(v) => set("folder", v)}
        placeholder="x-console-backups"
      />
    </>
  );
}

/** Google Drive：没授权时填客户端并授权，授权后显示账号。 */
export function GdriveFields({
  value,
  onChange,
  saved,
  authorizing,
  onAuthorize,
}: {
  value: GdriveForm;
  onChange: (next: GdriveForm) => void;
  saved?: BackupSettings["gdrive"];
  authorizing: boolean;
  onAuthorize: () => void;
}) {
  const t = useT();
  const revoke = useRevokeGdriveAuth();
  const set = <K extends keyof GdriveForm>(k: K, v: GdriveForm[K]) =>
    onChange({ ...value, [k]: v });
  const redirect = gdriveRedirectUri();
  const onCopy = async () => {
    if (await copyText(redirect)) toast(t("Copied"));
  };
  const onRevoke = async () => {
    const ok = await confirmAction({
      title: "撤销 Google Drive 授权？",
      description: "撤销后自动备份传不上去，已经传上去的备份还在网盘里。",
      confirmLabel: t("Revoke authorization"),
    });
    if (!ok) return;
    revoke.mutate(undefined, {
      onSuccess: () => toast("已撤销授权"),
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };
  const fields = (
    <TextField
      label={t("Folder name")}
      value={value.folderName}
      onChange={(v) => set("folderName", v)}
      placeholder="X Console 备份"
      hint="放在网盘根目录下。面板只能看到它自己建的文件。"
    />
  );
  if (saved?.authorized) {
    return (
      <>
        <div className="backup-gdrive-status">
          <span className="xc-dot ok" />
          <span>已授权{saved.account ? `：${saved.account}` : ""}</span>
          <button
            type="button"
            className="xc-btn"
            disabled={revoke.isPending}
            onClick={onRevoke}
          >
            {t("Revoke authorization")}
          </button>
        </div>
        {fields}
        <small className="backup-hint">
          Google 同意屏幕要发布成正式版，不然 7 天后授权会过期。
        </small>
      </>
    );
  }
  return (
    <>
      <ol className="backup-steps">
        <li>在 Google Cloud 控制台建一个项目，启用 Google Drive API。</li>
        <li>配置 OAuth 同意屏幕：用户类型选“外部”，再点“发布应用”。</li>
        <li>
          建一个“Web 应用”类型的 OAuth 客户端，已获授权的重定向 URI 填：
          <span className="backup-redirect">
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
        <li>把客户端 ID 和密钥填到下面，点“授权”，在 Google 页面同意。</li>
      </ol>
      <TextField
        label={t("Client ID")}
        value={value.clientId}
        onChange={(v) => set("clientId", v)}
        placeholder="xxxx.apps.googleusercontent.com"
      />
      <TextField
        label={t("Client secret")}
        type="password"
        value={value.clientSecret}
        onChange={(v) => set("clientSecret", v)}
        placeholder={saved?.secretSet ? "已保存，不改就留空" : ""}
      />
      {fields}
      <small className="backup-hint">
        Google 同意屏幕要发布成正式版，不然 7 天后授权会过期。没经过 Google
        审核也能发布，授权时点“继续”就行。
      </small>
      <div>
        <button
          type="button"
          className="xc-btn"
          disabled={authorizing}
          onClick={onAuthorize}
        >
          {t("Authorize")}
        </button>
      </div>
    </>
  );
}
