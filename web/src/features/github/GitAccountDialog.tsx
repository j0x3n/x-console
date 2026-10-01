import { useState } from "react";
import Dialog from "../../components/ui/Dialog";
import { CopyButton } from "../../components/ui/ErrorNotices";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useConnectionMutations,
  type GitConnection,
  type NewConnection,
} from "../aiagents/api";
import "../aiagents/i18n";
import "./i18n";

/*
 * Git 账号的弹窗（B62）：新建、改令牌、看 webhook。原来在 Agent 的
 * “Git 连接”页，现在设置 → Git 与 GitHub 用它，Agent 那边也从这里引用。
 */

/** 新建成功后显示 webhook 地址和密钥。 */
export function WebhookDialog({
  hook,
  onClose,
}: {
  hook: { url: string; secret: string };
  onClose: () => void;
}) {
  const t = useT();
  return (
    <Dialog open onClose={onClose} title={t("Webhook")}>
      <p className="xc-muted">
        {t(
          "Add a webhook for pull requests in the repository settings, so merged pull requests move their cards to done. The content type is application/json.",
        )}
      </p>
      <div className="aiagent-secret">
        <span>{t("Address")}</span>
        <code className="xc-mono">{hook.url}</code>
        <CopyButton text={hook.url} />
      </div>
      <div className="aiagent-secret">
        <span>{t("Secret")}</span>
        <code className="xc-mono">{hook.secret}</code>
        <CopyButton text={hook.secret} />
      </div>
      <div className="xc-dialog-actions">
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

export function TokenDialog({
  conn,
  onClose,
}: {
  conn: GitConnection;
  onClose: () => void;
}) {
  const t = useT();
  const ops = useConnectionMutations();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  return (
    <Dialog open onClose={onClose} title={`${t("Change token")}：${conn.name}`}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            const c = await ops.update.mutateAsync({
              id: conn.id,
              body: { token },
            });
            toast(c.lastError ? t("Saved, but the check failed") : t("Saved"));
            onClose();
          } catch (err) {
            setError(errorMessage(err));
          }
        }}
      >
        <label className="xc-field">
          <span>{t("Access token")}</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="off"
            value={token}
            required
            autoFocus
            onChange={(e) => setToken(e.target.value)}
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!token || ops.update.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export default function GitAccountDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (c: GitConnection, webhookSecret: string) => void;
}) {
  const t = useT();
  const ops = useConnectionMutations();
  const [kind, setKind] = useState<NewConnection["kind"]>("github");
  const [name, setName] = useState("GitHub");
  const [baseUrl, setBaseUrl] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const github = kind === "github";
  return (
    <Dialog open onClose={onClose} title={t("New Git account")} wide>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setError("");
          try {
            const body: NewConnection = { kind, name: name.trim() };
            if (baseUrl.trim()) body.baseUrl = baseUrl.trim();
            body.token = token.trim();
            const r = await ops.create.mutateAsync(body);
            toast(
              r.connection.lastError
                ? t("Saved, but the check failed")
                : t("Saved"),
            );
            onCreated(r.connection, r.webhookSecret);
          } catch (err) {
            setError(errorMessage(err));
          }
        }}
      >
        <div className="xc-field">
          <span>{t("Type")}</span>
          <div className="aiagent-kinds aiagent-kinds-row">
            {(["github", "forgejo"] as const).map((k) => (
              <label key={k} className="elevation-mode">
                <input
                  type="radio"
                  name="kind"
                  checked={kind === k}
                  onChange={() => {
                    setKind(k);
                    setName(k === "github" ? "GitHub" : "Forgejo");
                  }}
                />
                <span>
                  <strong>
                    {k === "github" ? "GitHub" : "Forgejo / Gitea"}
                  </strong>
                </span>
              </label>
            ))}
          </div>
        </div>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={60}
            required
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Address")}</span>
          <input
            className="xc-input"
            value={baseUrl}
            required={!github}
            placeholder={
              github ? "https://api.github.com" : "https://git.example.com"
            }
            onChange={(e) => setBaseUrl(e.target.value)}
          />
          <small>
            {github
              ? t(
                  "Leave empty for github.com. GitHub Enterprise: its API address.",
                )
              : t("The address you open Forgejo with.")}
          </small>
        </label>
        <label className="xc-field">
          <span>{t("Access token")}</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="off"
            value={token}
            required
            onChange={(e) => setToken(e.target.value)}
          />
          <small>
            {github
              ? t(
                  "Fine-grained token: read and write for Contents and Pull requests. The GitHub page also needs read access to Issues, Actions and Commit statuses.",
                )
              : t("A token with the write:repository scope.")}
          </small>
        </label>
        <small className="xc-muted">
          {t("The token is stored encrypted and never shown again.")}
        </small>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!name.trim() || ops.create.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
