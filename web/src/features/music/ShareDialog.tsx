import { Copy, Link as LinkIcon, Trash2 } from "lucide-react";
import { useState } from "react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, relativeTime } from "../../lib/time";
import { useCreateShare, useDeleteShare, useShares, type Share } from "./api";

const EXPIRY = [0, 1, 7, 30, 90];

/** 链接的完整地址：面板的地址加上路径。 */
export function shareUrl(share: Pick<Share, "path">): string {
  return `${window.location.origin}${share.path}`;
}

async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/**
 * 分享播放列表（B149）：生成公开链接，可以设密码和有效期。
 * 链接只能播这个播放列表里的歌，撤销后立即失效。
 */
export default function ShareDialog({
  playlistId,
  name,
  onClose,
}: {
  playlistId: number;
  name: string;
  onClose: () => void;
}) {
  const t = useT();
  const shares = useShares(playlistId);
  const create = useCreateShare(playlistId);
  const remove = useDeleteShare(playlistId);
  const [password, setPassword] = useState("");
  const [days, setDays] = useState(0);

  const copyLink = async (share: Share) => {
    toast((await copy(shareUrl(share))) ? t("Link copied") : shareUrl(share));
  };

  return (
    <Dialog
      open
      onClose={onClose}
      wide
      title={t("Share playlist")}
      description={name}
    >
      <form
        className="music-share-form"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate(
            {
              password: password || undefined,
              expiresInDays: days || undefined,
            },
            {
              onSuccess: async (share) => {
                setPassword("");
                toast(
                  (await copy(shareUrl(share)))
                    ? t("Link created and copied")
                    : t("Share link created"),
                );
              },
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            },
          );
        }}
      >
        <div className="xc-field">
          <label htmlFor="music-share-password">
            {t("Password (optional)")}
          </label>
          <input
            id="music-share-password"
            className="xc-input"
            type="text"
            autoComplete="off"
            maxLength={64}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <div className="xc-field">
          <label htmlFor="music-share-days">{t("Valid for")}</label>
          <select
            id="music-share-days"
            className="xc-select"
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          >
            {EXPIRY.map((d) => (
              <option key={d} value={d}>
                {d === 0 ? t("No end date") : `${d} ${t("days")}`}
              </option>
            ))}
          </select>
        </div>
        <button className="xc-btn primary" disabled={create.isPending}>
          <LinkIcon size={14} /> {t("Create link")}
        </button>
      </form>
      <p className="music-muted">
        {t(
          "Anyone with the link can play the songs in this playlist, and nothing else.",
        )}
      </p>
      <h4 className="music-subhead">{t("Share links")}</h4>
      {shares.isPending && <Loading />}
      {shares.isSuccess && shares.data.length === 0 && (
        <p className="music-muted">{t("No links yet")}</p>
      )}
      <ul className="music-share-list">
        {(shares.data ?? []).map((s) => (
          <li key={s.id}>
            <div className="music-row-main">
              <span className="music-row-title">{shareUrl(s)}</span>
              <span className="music-row-artist">
                {s.hasPassword ? `${t("Password")} · ` : ""}
                {s.expiresAt
                  ? `${t("Until")} ${formatDate(s.expiresAt)}`
                  : t("No end date")}{" "}
                · {s.viewCount} {t("visits")} · {relativeTime(s.createdAt)}
              </span>
            </div>
            <button
              type="button"
              className="xc-btn small"
              onClick={() => void copyLink(s)}
            >
              <Copy size={13} /> {t("Copy")}
            </button>
            <button
              type="button"
              className="xc-btn small danger"
              disabled={remove.isPending}
              onClick={async () => {
                if (
                  await confirmAction({
                    title: t("Revoke this link?"),
                    description: t(
                      "People who have it can no longer play the playlist.",
                    ),
                    confirmLabel: t("Revoke link"),
                  })
                )
                  remove.mutate(s.id, {
                    onError: (err) =>
                      toast({ message: errorMessage(err), tone: "error" }),
                  });
              }}
            >
              <Trash2 size={13} /> {t("Revoke link")}
            </button>
          </li>
        ))}
      </ul>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}
