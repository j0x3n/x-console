import { useEffect, useState, type CSSProperties, type FormEvent } from "react";
import { Copy, KeyRound, Plus, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import {
  useAddRemoteCalendars,
  useCalendarAccounts,
  useDeleteCalendarAccount,
  useRemoteCalendars,
  useSaveCalendarAccount,
  useStartCalendarAuth,
  type CalendarAccount,
} from "./api";

/* B215：Google 日历。账号授权后勾选要同步的日历，同步和写回走 CalDAV。 */

/** 日历管理页上方的 Google 账号列表，没有账号时不显示。 */
export function GoogleAccountsList({
  onAuthorize,
  onPick,
}: {
  onAuthorize: (account: CalendarAccount) => void;
  onPick: (id: number) => void;
}) {
  const t = useT();
  const accounts = useCalendarAccounts();
  const remove = useDeleteCalendarAccount();
  if (!accounts.data?.accounts.length) return null;
  return (
    <div className="xc-card calendar-list">
      {accounts.data.accounts.map((a) => (
        <div key={a.id} className="calendar-item">
          <div className="calendar-item-main">
            <div className="calendar-item-title">
              <strong>{a.email || t("Google account")}</strong>
              <span className="xc-badge">Google</span>
              {!a.authorized ? (
                <span className="xc-badge warn">{t("Not authorized")}</span>
              ) : a.authError ? (
                <span className="xc-badge danger">{t("Expired")}</span>
              ) : null}
            </div>
            <div className="calendar-item-meta">
              {a.authError ? (
                <span className="calendar-error">{a.authError}</span>
              ) : (
                <span>
                  {a.calendarCount} {t("calendars")}
                </span>
              )}
            </div>
          </div>
          <div className="calendar-item-actions">
            {a.authorized && !a.authError && (
              <button
                className="xc-btn ghost small"
                onClick={() => onPick(a.id)}
              >
                <Plus size={14} /> {t("Add calendars")}
              </button>
            )}
            <button
              className="xc-btn ghost small"
              onClick={() => onAuthorize(a)}
            >
              <KeyRound size={14} />{" "}
              {a.authorized ? t("Authorize again") : t("Authorize")}
            </button>
            <button
              className="xc-btn ghost small"
              title={t("Delete")}
              aria-label={t("Delete")}
              disabled={remove.isPending}
              onClick={async () => {
                if (
                  !(await confirmAction({
                    title: t(
                      "Delete this Google account and its synced calendars?",
                    ),
                    description: t(
                      "Events on Google are not touched. Only the copies here are removed.",
                    ),
                  }))
                )
                  return;
                remove.mutate(a.id, {
                  onSuccess: () => toast(t("Deleted")),
                  onError: (err) =>
                    toast({ message: errorMessage(err), tone: "error" }),
                });
              }}
            >
              <Trash2 size={14} />
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}

/** Google 授权需要的客户端 ID 和密钥，保存后跳到 Google 授权。 */
export function GoogleAccountForm({
  account,
  onCancel,
}: {
  account: CalendarAccount | null;
  onCancel: () => void;
}) {
  const t = useT();
  const accounts = useCalendarAccounts();
  const save = useSaveCalendarAccount();
  const startAuth = useStartCalendarAuth();
  const [clientId, setClientId] = useState(account?.clientId ?? "");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const redirect =
    accounts.data?.redirectUri ??
    `${window.location.origin}/api/v1/calendar/google/callback`;

  useEffect(() => {
    setClientId(account?.clientId ?? "");
    setSecret("");
    setError("");
  }, [account]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    if (!clientId.trim()) return setError(t("Client ID is required"));
    if (!account && !secret.trim())
      return setError(t("Client secret is required"));
    try {
      const body = {
        clientId: clientId.trim(),
        clientSecret: secret.trim() || undefined,
      };
      const saved = await save.mutateAsync({ id: account?.id, body });
      const r = await startAuth.mutateAsync(saved.id);
      window.location.href = r.url;
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const onCopy = async () => {
    if (await copyText(redirect)) toast(t("Copied"));
  };

  return (
    <form className="xc-stack" onSubmit={submit}>
      <ol className="calendar-steps">
        <li>
          在 Google Cloud 控制台建一个项目，启用 Google Calendar API 和 CalDAV
          API。云盘用过的项目也可以，多启用这两个。
        </li>
        <li>配置 OAuth 同意屏幕：用户类型选“外部”，再点“发布应用”。</li>
        <li>
          建一个“Web 应用”类型的 OAuth 客户端，已获授权的重定向 URI 填：
          <span className="calendar-redirect">
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
        <li>把客户端 ID 和密钥填到下面，保存后在 Google 页面同意授权。</li>
      </ol>
      <label className="xc-field">
        <span>{t("Client ID")}</span>
        <input
          className="xc-input"
          value={clientId}
          autoComplete="off"
          placeholder="xxxx.apps.googleusercontent.com"
          onChange={(e) => setClientId(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Client secret")}</span>
        <input
          className="xc-input"
          type="password"
          value={secret}
          autoComplete="new-password"
          placeholder={
            account?.secretSet ? t("Saved, leave empty to keep") : ""
          }
          onChange={(e) => setSecret(e.target.value)}
        />
        <small>
          同意屏幕要发布成正式版，不然 7 天后授权会过期。没经过 Google
          审核也能发布。
        </small>
      </label>
      {error && <p className="xc-error-text">{error}</p>}
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onCancel}>
          {t("Cancel")}
        </button>
        <button
          className="xc-btn primary"
          disabled={save.isPending || startAuth.isPending}
        >
          {t("Save and authorize")}
        </button>
      </div>
    </form>
  );
}

/** 授权后勾选要同步的 Google 日历。 */
export function RemoteCalendarsDialog({
  accountId,
  onClose,
}: {
  accountId: number | null;
  onClose: () => void;
}) {
  const t = useT();
  const remote = useRemoteCalendars(accountId);
  const add = useAddRemoteCalendars();
  const [picked, setPicked] = useState<string[]>([]);

  useEffect(() => {
    if (!remote.data) return;
    // 第一次打开时默认勾上主日历
    const fresh = remote.data.calendars.filter((c) => !c.added);
    const primary = fresh.find((c) => c.primary);
    setPicked(primary ? [primary.id] : []);
  }, [remote.data]);

  const toggle = (id: string, on: boolean) =>
    setPicked((cur) => (on ? [...cur, id] : cur.filter((x) => x !== id)));

  const submit = () => {
    if (accountId === null || picked.length === 0) return;
    add.mutate(
      { id: accountId, ids: picked },
      {
        onSuccess: (res) => {
          toast(
            t("{n} calendars added. Syncing now.").replace(
              "{n}",
              String(res.calendars.length),
            ),
          );
          onClose();
        },
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  return (
    <Dialog
      open={accountId !== null}
      onClose={onClose}
      title={t("Choose Google calendars")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={add.isPending || picked.length === 0}
            onClick={submit}
          >
            {t("Add")}
          </button>
        </>
      }
    >
      {remote.isPending ? (
        <Loading />
      ) : remote.isError ? (
        <ErrorState error={remote.error} onRetry={() => remote.refetch()} />
      ) : (
        <div className="xc-stack calendar-remote-list">
          {remote.data.calendars.map((c) => (
            <label key={c.id} className="xc-check">
              <input
                type="checkbox"
                checked={c.added || picked.includes(c.id)}
                disabled={c.added}
                onChange={(e) => toggle(c.id, e.target.checked)}
              />
              <span className="calendar-remote-name">
                <span
                  className="calendar-dot"
                  style={{ "--ev": c.color || undefined } as CSSProperties}
                />
                {c.name}
                {c.primary && <span className="xc-badge">{t("Primary")}</span>}
                {c.readOnly && (
                  <span className="xc-badge">{t("Read only")}</span>
                )}
                {c.added && (
                  <span className="xc-badge ok">{t("Already added")}</span>
                )}
              </span>
            </label>
          ))}
        </div>
      )}
    </Dialog>
  );
}
