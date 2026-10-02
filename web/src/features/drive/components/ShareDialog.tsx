import { useState } from "react";
import { Copy, Dices, History, Link2, Trash2 } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { Segmented } from "../../../components/ui/Toolbar";
import { ErrorState, Loading, NotLive } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  isNotLive,
  useCreateShare,
  useDeleteShare,
  useDriveShares,
  type DriveItem,
  type DriveShare,
  type DriveShareInput,
} from "../api";
import { randomCode, shareText, validShareCode } from "../logic";
import ShareDownloads from "./ShareDownloads";

type Expiry = DriveShareInput["expiresIn"];

/** 复制分享链接和密码。 */
export function copyShare(share: DriveShare, t: (s: string) => string) {
  return navigator.clipboard
    .writeText(shareText(share))
    .then(() => toast(t("Link copied")))
    .catch(() => toast({ message: t("Could not copy"), tone: "error" }));
}

/** 取消一条分享链接，先确认。 */
export async function cancelShare(
  share: DriveShare,
  remove: (id: number) => void,
  t: (s: string) => string,
) {
  if (
    await confirmAction({
      title: `${t("Cancel share of")}“${share.itemName}”？`,
      description: "链接马上失效，已经打开的人也不能再下载。",
      confirmLabel: t("Cancel share"),
      danger: true,
    })
  )
    remove(share.id);
}

/**
 * 分享（B31）：给一个文件或文件夹建外链。
 * 选有效期、密码、下载次数上限。下面列出这个条目已有的链接，可以复制和取消，
 * 能展开看最近的下载记录（B75）。
 */
export default function ShareDialog({
  item,
  onClose,
}: {
  item: DriveItem;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const shares = useDriveShares(item.id);
  const create = useCreateShare();
  const remove = useDeleteShare();
  const [expiresIn, setExpiresIn] = useState<Expiry>("7d");
  const [useCode, setUseCode] = useState(true);
  const [code, setCode] = useState(() => randomCode(6));
  const [open, setOpen] = useState<number | null>(null);
  const [limit, setLimit] = useState(false);
  const [maxDownloads, setMaxDownloads] = useState(10);
  const codeOk = !useCode || validShareCode(code.trim());

  const submit = () =>
    create.mutate(
      {
        itemId: item.id,
        expiresIn,
        code: useCode ? code.trim() : undefined,
        maxDownloads: limit ? maxDownloads : undefined,
      },
      {
        onSuccess: (share) => {
          void copyShare(share, t);
          setCode(randomCode(6));
        },
      },
    );

  if (shares.isError && isNotLive(shares.error))
    return (
      <Dialog open onClose={onClose} title={t("Share")} description={item.name}>
        <NotLive name={t("Share links")} icon={<Link2 size={28} />} />
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    );

  const active = (shares.data ?? []).filter((s) => s.active);

  return (
    <Dialog open onClose={onClose} title={t("Share")} description={item.name}>
      <form
        className="drive-share-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (codeOk) submit();
        }}
      >
        <div className="xc-field">
          <span>{t("Valid for")}</span>
          <Segmented
            label={t("Valid for")}
            value={expiresIn}
            onChange={setExpiresIn}
            options={[
              { value: "1d", label: t("1 day") },
              { value: "7d", label: t("7 days") },
              { value: "30d", label: t("30 days") },
              { value: "never", label: t("Forever") },
            ]}
          />
        </div>
        <div className="drive-share-row">
          <label className="xc-check">
            <input
              type="checkbox"
              checked={useCode}
              onChange={(e) => setUseCode(e.target.checked)}
            />
            {t("Password")}
          </label>
          {useCode && (
            <>
              <input
                className="xc-input drive-share-code"
                value={code}
                maxLength={32}
                aria-label={t("Password")}
                aria-invalid={!codeOk}
                onChange={(e) => setCode(e.target.value)}
              />
              <button
                type="button"
                className="xc-btn ghost small"
                title={t("Random password")}
                onClick={() => setCode(randomCode(6))}
              >
                <Dices size={14} />
                <span className="drive-btn-text">{t("Random password")}</span>
              </button>
            </>
          )}
        </div>
        {useCode && !codeOk && (
          <small className="drive-share-error">{t("4 to 32 characters")}</small>
        )}
        <div className="drive-share-row">
          <label className="xc-check">
            <input
              type="checkbox"
              checked={limit}
              onChange={(e) => setLimit(e.target.checked)}
            />
            {t("Limit downloads")}
          </label>
          {limit && (
            <input
              className="xc-input drive-share-count"
              type="number"
              min={1}
              value={maxDownloads}
              aria-label={t("Max downloads")}
              onChange={(e) =>
                setMaxDownloads(Math.max(1, Number(e.target.value) || 1))
              }
            />
          )}
        </div>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Close")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={create.isPending || !codeOk}
          >
            <Link2 size={14} /> {t("Create link")}
          </button>
        </div>
      </form>

      <section className="drive-share-list" aria-label={t("Existing links")}>
        <h3>{t("Existing links")}</h3>
        {shares.isPending ? (
          <Loading />
        ) : shares.isError ? (
          <ErrorState error={shares.error} onRetry={() => shares.refetch()} />
        ) : active.length === 0 ? (
          <p className="drive-muted">{t("No links yet")}</p>
        ) : (
          <ul>
            {active.map((s) => (
              <li key={s.id}>
                <span className="drive-share-url" title={s.url}>
                  {s.url}
                </span>
                <small className="drive-muted">
                  {s.code ? `${t("Password")} ${s.code} · ` : ""}
                  {s.expiresAt
                    ? `${relativeTime(s.expiresAt, language)}${t("expires")}`
                    : t("Never expires")}
                  {` · ${t("Opened times")} ${s.visits}`}
                  {` · ${t("Downloads")} ${s.downloads}`}
                  {s.maxDownloads ? ` / ${s.maxDownloads}` : ""}
                  {s.lastAccessAt &&
                    ` · ${t("Last opened")} ${relativeTime(s.lastAccessAt, language)}`}
                </small>
                <span className="drive-share-actions">
                  <button
                    type="button"
                    className="xc-btn ghost small"
                    title={t("Download history")}
                    aria-label={t("Download history")}
                    aria-expanded={open === s.id}
                    onClick={() => setOpen(open === s.id ? null : s.id)}
                  >
                    <History size={14} />
                  </button>
                  <button
                    type="button"
                    className="xc-btn ghost small"
                    title={t("Copy link")}
                    aria-label={t("Copy link")}
                    onClick={() => void copyShare(s, t)}
                  >
                    <Copy size={14} />
                  </button>
                  <button
                    type="button"
                    className="xc-btn ghost small danger"
                    title={t("Cancel share")}
                    aria-label={t("Cancel share")}
                    onClick={() => void cancelShare(s, remove.mutate, t)}
                  >
                    <Trash2 size={14} />
                  </button>
                </span>
                {open === s.id && (
                  <ShareDownloads shareId={s.id} isDir={s.isDir} />
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </Dialog>
  );
}
