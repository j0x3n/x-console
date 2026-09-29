import { useState } from "react";
import { Copy, Link2, Trash2 } from "lucide-react";
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
import { randomCode, shareText } from "../logic";

type Expiry = DriveShareInput["expiresIn"];

/** 复制分享链接和提取码。 */
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
 * 选有效期、提取码、下载次数上限。下面列出这个条目已有的链接，可以复制和取消。
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
  const [code, setCode] = useState(() => randomCode());
  const [limit, setLimit] = useState(false);
  const [maxDownloads, setMaxDownloads] = useState(10);
  const codeOk = !useCode || /^[A-Za-z0-9]{4,8}$/.test(code);

  const submit = () =>
    create.mutate(
      {
        itemId: item.id,
        expiresIn,
        code: useCode ? code : undefined,
        maxDownloads: limit ? maxDownloads : undefined,
      },
      {
        onSuccess: (share) => {
          void copyShare(share, t);
          setCode(randomCode());
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
            {t("Access code")}
          </label>
          {useCode && (
            <input
              className="xc-input drive-share-code"
              value={code}
              maxLength={8}
              aria-label={t("Access code")}
              aria-invalid={!codeOk}
              onChange={(e) => setCode(e.target.value.trim())}
            />
          )}
        </div>
        {useCode && !codeOk && (
          <small className="drive-share-error">
            {t("4 to 8 letters or digits")}
          </small>
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
                  {s.code ? `${t("Access code")} ${s.code} · ` : ""}
                  {s.expiresAt
                    ? `${relativeTime(s.expiresAt, language)}${t("expires")}`
                    : t("Never expires")}
                  {` · ${t("Downloads")} ${s.downloads}`}
                  {s.maxDownloads ? ` / ${s.maxDownloads}` : ""}
                </small>
                <span className="drive-share-actions">
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
              </li>
            ))}
          </ul>
        )}
      </section>
    </Dialog>
  );
}
