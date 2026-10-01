import { useEffect, useState } from "react";
import { Copy, Link2, Link2Off } from "lucide-react";
import { errorMessage, isNotLive } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { Loading, NotLive } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  useDeleteNoteShare,
  useNoteShare,
  useSaveNoteShare,
  type NoteShareInput,
} from "../api";

type Expires = NoteShareInput["expiresIn"];

const EXPIRES: { id: Expires; label: string }[] = [
  { id: "1d", label: "1 day" },
  { id: "7d", label: "7 days" },
  { id: "30d", label: "30 days" },
  { id: "never", label: "Never expires" },
];

/**
 * 笔记外链（B72）。外链打开只有笔记内容，可以设密码。
 * 已分享时显示链接、打开次数，可以改密码、改有效期、停止分享。
 */
export default function ShareDialog({
  noteId,
  onClose,
}: {
  noteId: number;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const share = useNoteShare(noteId);
  const save = useSaveNoteShare(noteId);
  const remove = useDeleteNoteShare(noteId);
  const [expires, setExpires] = useState<Expires>("7d");
  const [usePassword, setUsePassword] = useState(false);
  const [password, setPassword] = useState("");
  useEffect(() => {
    if (share.data) setUsePassword(share.data.hasPassword);
  }, [share.data]);

  const current = share.data;
  const passwordOk =
    !usePassword ||
    (password.length >= 4 && password.length <= 32) ||
    (!!current?.hasPassword && password === "");

  const submit = () => {
    const body: NoteShareInput = { expiresIn: expires };
    if (usePassword && password) body.password = password;
    if (!usePassword && current?.hasPassword) body.clearPassword = true;
    save.mutate(body, {
      onSuccess: (s) => {
        setPassword("");
        toast(current ? t("Saved") : t("Link created"));
        if (!current) void copy(s.url);
      },
      onError: (error) =>
        toast({ message: errorMessage(error), tone: "error" }),
    });
  };
  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      toast(t("Link copied"));
    } catch {
      toast({ message: t("Could not copy. Copy it by hand."), tone: "error" });
    }
  };
  const stop = async () => {
    if (
      await confirmAction({
        title: t("Stop sharing this note?"),
        description: t("The link stops working right away."),
        confirmLabel: t("Stop sharing"),
      })
    )
      remove.mutate(undefined, {
        onSuccess: () => toast(t("Sharing stopped")),
      });
  };

  let body;
  if (share.isPending) body = <Loading />;
  else if (share.isError)
    body = isNotLive(share.error) ? (
      <NotLive name={t("Note sharing")} icon={<Link2 size={28} />} />
    ) : (
      <p className="xc-error-text">{errorMessage(share.error)}</p>
    );
  else
    body = (
      <div className="xc-stack">
        {current && (
          <div className="notes-share-current">
            <div className="notes-share-url">
              <Link2 size={14} />
              <input
                className="xc-input xc-mono"
                readOnly
                value={current.url}
                aria-label={t("Link")}
                onFocus={(e) => e.target.select()}
              />
              <button
                type="button"
                className="xc-btn small"
                onClick={() => void copy(current.url)}
              >
                <Copy size={13} /> {t("Copy")}
              </button>
            </div>
            <small className="xc-muted">
              {current.expiresAt
                ? `${relativeTime(current.expiresAt, language)}${t("expires")}`
                : t("Never expires")}
              {" · "}
              {current.hasPassword ? t("Password set") : t("No password")}
              {" · "}
              {t("Opened times")} {current.visits}
              {current.lastVisitAt &&
                ` · ${t("Last opened")} ${relativeTime(current.lastVisitAt, language)}`}
            </small>
          </div>
        )}
        <label className="xc-field">
          <span>
            {current ? t("Change how long it works") : t("Valid for")}
          </span>
          <select
            className="xc-select"
            value={expires}
            onChange={(e) => setExpires(e.target.value as Expires)}
          >
            {EXPIRES.map((x) => (
              <option key={x.id} value={x.id}>
                {t(x.label)}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={usePassword}
            onChange={(e) => setUsePassword(e.target.checked)}
          />
          <span>{t("Require a password")}</span>
        </label>
        {usePassword && (
          <label className="xc-field">
            <span>{t("Password")}</span>
            <input
              className="xc-input"
              type="text"
              autoComplete="off"
              value={password}
              maxLength={32}
              placeholder={
                current?.hasPassword
                  ? t("Leave empty to keep the old password")
                  : t("4 to 32 characters")
              }
              onChange={(e) => setPassword(e.target.value)}
            />
            <small>
              {t(
                "People need this password to open the link. Send it separately.",
              )}
            </small>
          </label>
        )}
        <small className="xc-muted">
          {t(
            "The link shows only this note: the title, the text and its images. Hidden notes cannot be shared.",
          )}
        </small>
      </div>
    );

  return (
    <Dialog open onClose={onClose} title={t("Share note")}>
      {body}
      <div className="xc-dialog-actions">
        {current && (
          <button
            type="button"
            className="xc-btn ghost danger"
            onClick={() => void stop()}
            disabled={remove.isPending}
          >
            <Link2Off size={14} /> {t("Stop sharing")}
          </button>
        )}
        <span className="xc-spacer" />
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
        {!share.isError && !share.isPending && (
          <button
            type="button"
            className="xc-btn primary"
            disabled={!passwordOk || save.isPending}
            onClick={submit}
          >
            {current ? t("Save") : t("Create link")}
          </button>
        )}
      </div>
    </Dialog>
  );
}
