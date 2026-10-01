import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import {
  Cloud,
  KeyRound,
  Pencil,
  PlugZap,
  Plus,
  Trash2,
  Unplug,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu, { type MoreMenuItem } from "../../components/ui/MoreMenu";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDeleteRemote,
  useRemotes,
  useRevokeRemoteAuth,
  useStartRemoteAuth,
  useTestRemote,
  type StorageRemote,
} from "./api";
import RemoteDialog from "./RemoteDialog";
import "./i18n";
import "./storage.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** Google 授权完跳回来时，地址里带着结果。提示一次，然后去掉参数。 */
function useGdriveReturn() {
  const [params, setParams] = useSearchParams();
  const result = params.get("gdrive");
  const message = params.get("message");
  useEffect(() => {
    if (!result) return;
    if (result === "ok") toast("Google Drive 已授权");
    else
      toast({
        message: `Google Drive 授权失败：${message || "未知原因"}`,
        tone: "error",
      });
    setParams(
      (p) => {
        p.delete("gdrive");
        p.delete("message");
        return p;
      },
      { replace: true },
    );
  }, [result, message, setParams]);
}

/** 设置 → 存储里的网盘账号（B69）。备份和云盘页都用这里的账号。 */
export default function RemotesCard() {
  const t = useT();
  const remotes = useRemotes();
  const [editing, setEditing] = useState<StorageRemote | "new" | null>(null);
  useGdriveReturn();
  const items = remotes.data?.items ?? [];
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Drive accounts")}</h2>
        <button
          className="xc-btn small"
          onClick={() => setEditing("new")}
          title={t("Add drive account")}
        >
          <Plus size={14} /> {t("Add drive account")}
        </button>
      </div>
      {remotes.isPending ? (
        <Loading />
      ) : remotes.isError ? (
        <ErrorState error={remotes.error} onRetry={() => remotes.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          title={t("No drive accounts yet")}
          icon={<Cloud size={26} />}
        >
          <span>
            加一个坚果云、Nextcloud 或 Google
            Drive。自动备份可以传到这里，云盘页也能浏览。
          </span>
        </EmptyState>
      ) : (
        <ul className="storage-remote-list">
          {items.map((r) => (
            <RemoteRow key={r.id} remote={r} onEdit={() => setEditing(r)} />
          ))}
        </ul>
      )}
      <RemoteDialog
        open={editing !== null}
        onClose={() => setEditing(null)}
        remote={editing === "new" ? null : editing}
      />
    </section>
  );
}

function RemoteRow({
  remote: r,
  onEdit,
}: {
  remote: StorageRemote;
  onEdit: () => void;
}) {
  const t = useT();
  const remove = useDeleteRemote();
  const test = useTestRemote();
  const startAuth = useStartRemoteAuth();
  const revoke = useRevokeRemoteAuth();
  const gdrive = r.kind === "gdrive";
  const detail = gdrive
    ? r.gdrive?.authorized
      ? (r.gdrive.account ?? t("Authorized"))
      : t("Not authorized")
    : [r.webdav?.username, r.webdav?.url].filter(Boolean).join(" · ");

  const authorize = () =>
    startAuth.mutate(r.id, {
      onSuccess: (res) => {
        window.location.href = res.url;
      },
      onError: fail,
    });

  const items: MoreMenuItem[] = [
    {
      key: "edit",
      label: t("Edit"),
      icon: <Pencil size={14} />,
      onSelect: onEdit,
    },
  ];
  if (!gdrive || r.gdrive?.authorized)
    items.push({
      key: "test",
      label: t("Test connection"),
      icon: <PlugZap size={14} />,
      onSelect: () =>
        test.mutate(
          { id: r.id, body: {} },
          {
            onSuccess: (res) =>
              toast({
                message: res.message,
                tone: res.ok ? undefined : "error",
              }),
            onError: fail,
          },
        ),
    });
  if (gdrive)
    items.push({
      key: "auth",
      label: r.gdrive?.authorized ? t("Authorize again") : t("Authorize"),
      icon: <KeyRound size={14} />,
      onSelect: authorize,
    });
  if (gdrive && r.gdrive?.authorized)
    items.push({
      key: "revoke",
      label: t("Revoke authorization"),
      icon: <Unplug size={14} />,
      onSelect: async () => {
        const ok = await confirmAction({
          title: `撤销“${r.name}”的授权？`,
          description:
            "撤销后用它的自动备份传不上去，网盘里已有的文件不受影响。",
          confirmLabel: t("Revoke authorization"),
        });
        if (ok) revoke.mutate(r.id, { onError: fail });
      },
    });
  items.push({
    key: "delete",
    label: t("Delete"),
    icon: <Trash2 size={14} />,
    danger: true,
    onSelect: async () => {
      if (r.usedByBackup) {
        toast({
          message: "自动备份正在用这个账号，先到 设置 → 备份 换一个备份位置",
          tone: "error",
        });
        return;
      }
      const ok = await confirmAction({
        title: `${t("Delete")}“${r.name}”？`,
        description: "只删除这里保存的账号，网盘里的文件不受影响。",
        confirmLabel: t("Delete"),
      });
      if (ok)
        remove.mutate(r.id, {
          onSuccess: () => toast(t("Deleted")),
          onError: fail,
        });
    },
  });

  return (
    <li className="storage-remote-row">
      <span className={`xc-dot ${r.ready ? "ok" : "warn"}`} />
      <span className="storage-remote-main">
        <strong>{r.name}</strong>
        <small>
          {gdrive ? "Google Drive" : "WebDAV"}
          {detail && ` · ${detail}`}
        </small>
        {gdrive && r.gdrive?.limited && (
          <small className="storage-remote-warn">
            {t("Only sees files the panel made. Authorize again to see all.")}
          </small>
        )}
      </span>
      {r.usedByBackup && (
        <span className="xc-badge info">{t("Used by backup")}</span>
      )}
      {!r.showInDrive && (
        <span className="xc-badge">{t("Hidden on the drive page")}</span>
      )}
      {gdrive && !r.gdrive?.authorized && (
        <button
          className="xc-btn small"
          disabled={startAuth.isPending}
          onClick={authorize}
        >
          {t("Authorize")}
        </button>
      )}
      <MoreMenu
        label={`${t("More")}：${r.name}`}
        title={r.name}
        items={items}
      />
    </li>
  );
}
