import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useSaveMailAccount,
  type MailAccount,
  type MailProvider,
} from "../api";
import { guessProvider, providerPresets } from "../logic";
import { useNotifyMutes, useReplaceScopeMutes } from "../../reminders/api";
import NotifyTargetsField from "../../reminders/NotifyTargets";
import { mailScope, sameTargets } from "../../reminders/mutes";

/** 新邮件通知的类型。静音规则按它加邮箱范围保存（B113）。 */
const MAIL_KIND = "mail.new";

const providers: { id: MailProvider; label: string }[] = [
  { id: "gmail", label: "Gmail" },
  { id: "aliyun", label: "Aliyun enterprise mail" },
  { id: "other", label: "Other IMAP" },
];

/** 添加或修改邮箱（B53）。保存时服务端先试着登录，失败不保存。 */
export default function AccountDialog({
  open,
  onClose,
  account,
}: {
  open: boolean;
  onClose: () => void;
  account?: MailAccount | null;
}) {
  const t = useT();
  const save = useSaveMailAccount();
  const [provider, setProvider] = useState<MailProvider>("gmail");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("993");
  const [username, setUsername] = useState("");
  const [notify, setNotify] = useState(true);
  const [error, setError] = useState("");
  // B113：这个邮箱的新邮件不发到哪些设备或渠道
  const [muted, setMuted] = useState<string[]>([]);
  const [savedMuted, setSavedMuted] = useState<string[]>([]);
  const mutes = useNotifyMutes(
    account ? mailScope(account.id) : undefined,
    open && !!account,
  );
  const replaceMutes = useReplaceScopeMutes();

  useEffect(() => {
    if (!open) return;
    setError("");
    setPassword("");
    setProvider(account?.provider ?? "gmail");
    setName(account?.name ?? "");
    setEmail(account?.email ?? "");
    setHost(account?.imapHost ?? "");
    setPort(String(account?.imapPort ?? 993));
    setUsername(account?.username ?? "");
    setNotify(account?.notify ?? true);
    setMuted([]);
    setSavedMuted([]);
  }, [open, account]);

  useEffect(() => {
    if (!open || !account || !mutes.data) return;
    const targets = mutes.data
      .filter((m) => m.kindPattern === MAIL_KIND)
      .map((m) => m.target);
    setMuted(targets);
    setSavedMuted(targets);
  }, [open, account, mutes.data]);

  const pick = (p: MailProvider) => {
    setProvider(p);
    setHost(providerPresets[p].imapHost);
    setPort(String(providerPresets[p].imapPort));
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const mail = email.trim();
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(mail))
      return setError(t("Please enter a valid email address"));
    if (!account && !password) return setError(t("Please enter the password"));
    const imapHost =
      provider === "other" ? host.trim() : providerPresets[provider].imapHost;
    if (!imapHost) return setError(t("Please enter the IMAP server"));
    const imapPort = Number(port) || 993;
    try {
      let id = account?.id;
      if (account) {
        await save.mutateAsync({
          id: account.id,
          patch: {
            name: name.trim() || mail,
            notify,
            ...(provider === "other" ? { imapHost, imapPort } : {}),
            ...(username.trim() ? { username: username.trim() } : {}),
            ...(password ? { password } : {}),
          },
        });
      } else {
        const created = await save.mutateAsync({
          create: {
            name: name.trim() || mail,
            email: mail,
            provider,
            imapHost,
            imapPort,
            username: username.trim() || undefined,
            password,
            notify,
          },
        });
        id = (created as { id?: number } | undefined)?.id;
      }
      toast(t("Mailbox connected"));
      if (id && !sameTargets(muted, savedMuted)) {
        try {
          await replaceMutes.mutateAsync({
            kindPattern: MAIL_KIND,
            scope: mailScope(id),
            targets: muted,
          });
        } catch (err) {
          toast({ message: errorMessage(err), tone: "error" });
        }
      }
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={account ? t("Edit mailbox") : t("Add mailbox")}
    >
      <form className="mail-account-form" onSubmit={submit}>
        {!account && (
          <div className="xc-field">
            <span>{t("Mail provider")}</span>
            <div className="mail-providers" role="radiogroup">
              {providers.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  role="radio"
                  aria-checked={provider === p.id}
                  className={provider === p.id ? "active" : ""}
                  onClick={() => pick(p.id)}
                >
                  {t(p.label)}
                </button>
              ))}
            </div>
          </div>
        )}
        <label className="xc-field">
          <span>{t("Email address")}</span>
          <input
            className="xc-input"
            type="email"
            value={email}
            disabled={!!account}
            autoComplete="off"
            onChange={(e) => {
              setEmail(e.target.value);
              if (!account && provider === "other") {
                const g = guessProvider(e.target.value);
                if (g !== "other") pick(g);
              }
            }}
            placeholder="name@example.com"
            autoFocus={!account}
          />
        </label>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={50}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("For example: Personal Gmail")}
          />
        </label>
        <label className="xc-field">
          <span>
            {provider === "gmail" ? t("App password") : t("Password")}
          </span>
          <input
            className="xc-input"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={
              account ? t("Leave empty to keep the password") : undefined
            }
          />
          <small>{t(passwordHelp[provider])}</small>
        </label>
        {provider === "other" && (
          <div className="mail-form-row">
            <label className="xc-field">
              <span>{t("IMAP server")}</span>
              <input
                className="xc-input"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="imap.example.com"
                spellCheck={false}
              />
            </label>
            <label className="xc-field mail-port">
              <span>{t("Port")}</span>
              <input
                className="xc-input"
                inputMode="numeric"
                value={port}
                onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))}
              />
            </label>
          </div>
        )}
        <details className="mail-advanced">
          <summary>{t("Advanced")}</summary>
          <label className="xc-field">
            <span>{t("Login name")}</span>
            <input
              className="xc-input"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t("Same as the email address if empty")}
              autoComplete="off"
            />
          </label>
        </details>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={notify}
            onChange={(e) => setNotify(e.target.checked)}
          />
          <span>{t("Push new mail")}</span>
        </label>
        <div className="mail-notify-targets">
          {provider === "gmail" && (
            <small className="xc-muted">
              {t("Gmail is on your phone? Untick the phone here.")}
            </small>
          )}
          {notify ? (
            <NotifyTargetsField muted={muted} onChange={setMuted} />
          ) : (
            <small className="xc-muted">
              {t("Notifications of this mailbox are switched off everywhere.")}
            </small>
          )}
        </div>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={save.isPending}>
            {save.isPending ? t("Connecting…") : t("Connect")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

const passwordHelp: Record<MailProvider, string> = {
  gmail:
    "Gmail needs 2-Step Verification. Then create an app password in your Google account under Security → App passwords.",
  aliyun:
    "Turn on IMAP in the web mail settings first. If your company requires it, use a client password.",
  other: "The password or app password for IMAP login.",
};
