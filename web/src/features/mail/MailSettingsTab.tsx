import { useState } from "react";
import { Mail, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useDeleteMailAccount,
  useMailAccounts,
  useSyncMailAccount,
  type MailAccount,
} from "./api";
import AccountDialog from "./components/AccountDialog";
import { useNotifyMutes } from "../reminders/api";
import { mailScope } from "../reminders/mutes";
import "./i18n";
import "./mail.css";

/** 设置 → 邮件（B53）：邮箱账号的添加、修改、删除和连接状态。 */
export default function MailSettingsTab() {
  const t = useT();
  const accounts = useMailAccounts();
  const mutes = useNotifyMutes();
  const [editing, setEditing] = useState<MailAccount | "new" | null>(null);
  if (accounts.isPending) return <Loading />;
  if (accounts.isError)
    return isNotLive(accounts.error) ? (
      <NotLive name={t("Mail")} icon={<Mail size={28} />} />
    ) : (
      <ErrorState error={accounts.error} onRetry={() => accounts.refetch()} />
    );
  return (
    <div className="mail-settings">
      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Mailboxes")}</h2>
          <button
            className="xc-btn small"
            onClick={() => setEditing("new")}
            title={t("Add mailbox")}
          >
            <Plus size={14} /> {t("Add mailbox")}
          </button>
        </div>
        {accounts.data.length === 0 ? (
          <EmptyState title={t("No mailboxes yet")} icon={<Mail size={28} />}>
            <span>
              {t(
                "Add Gmail or an enterprise mailbox. New mail shows up here within seconds.",
              )}
            </span>
          </EmptyState>
        ) : (
          <ul className="mail-account-list">
            {accounts.data.map((a) => (
              <AccountRow
                key={a.id}
                account={a}
                onEdit={() => setEditing(a)}
                partlyMuted={(mutes.data ?? []).some(
                  (m) => m.scope === mailScope(a.id),
                )}
              />
            ))}
          </ul>
        )}
        <p className="mail-note">
          {t(
            "Passwords are stored encrypted with the master key. Only the inbox is synced. Sending mail is not supported yet.",
          )}
        </p>
      </div>
      <AccountDialog
        open={editing !== null}
        onClose={() => setEditing(null)}
        account={editing === "new" ? null : editing}
      />
    </div>
  );
}

function AccountRow({
  account: a,
  onEdit,
  partlyMuted,
}: {
  account: MailAccount;
  onEdit: () => void;
  /** B113：这个邮箱有静音规则 */
  partlyMuted: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const remove = useDeleteMailAccount();
  const sync = useSyncMailAccount();
  const tone =
    a.status === "ok" ? "ok" : a.status === "error" ? "danger" : "warn";
  return (
    <li className="mail-account-row">
      <span className={`xc-dot ${tone}`} />
      <span className="mail-account-main">
        <strong>{a.name}</strong>
        <small>
          {a.email} · {a.imapHost}
          {a.status === "ok" &&
            ` · ${a.idle ? t("Real-time") : t("Checked every minute")}`}
          {a.lastSyncAt &&
            ` · ${t("Last synced")} ${relativeTime(a.lastSyncAt, language)}`}
        </small>
        {a.status === "error" && a.lastError && (
          <small className="mail-account-error">{a.lastError}</small>
        )}
      </span>
      {!a.notify && <span className="xc-badge">{t("No push")}</span>}
      {a.notify && partlyMuted && (
        <span className="xc-badge">{t("Some targets muted")}</span>
      )}
      <MoreMenu
        label={`${t("More")}：${a.name}`}
        title={a.name}
        items={[
          {
            key: "edit",
            label: t("Edit"),
            icon: <Pencil size={14} />,
            onSelect: onEdit,
          },
          {
            key: "sync",
            label: t("Sync now"),
            icon: <RefreshCw size={14} />,
            onSelect: () =>
              sync.mutate(a.id, { onSuccess: () => toast(t("Syncing mail")) }),
          },
          {
            key: "delete",
            label: t("Delete"),
            icon: <Trash2 size={14} />,
            danger: true,
            onSelect: async () => {
              if (
                await confirmAction({
                  title: `${t("Delete mailbox")}“${a.name}”？`,
                  description: t(
                    "Mail cached here is removed. Mail in the mailbox itself is not touched.",
                  ),
                  confirmLabel: t("Delete"),
                })
              )
                remove.mutate(a.id, { onSuccess: () => toast(t("Deleted")) });
            },
          },
        ]}
      />
    </li>
  );
}
