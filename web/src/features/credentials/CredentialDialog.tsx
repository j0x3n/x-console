import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateCredential,
  useUpdateCredential,
  type CredentialItem,
  type CredentialKind,
} from "./api";
import {
  KINDS,
  KIND_LABELS,
  formatDays,
  parseDays,
  parseLines,
} from "./format";

interface Props {
  open: boolean;
  onClose: () => void;
  /** 传了就是修改 */
  credential?: CredentialItem | null;
  /** 新建时默认的类型 */
  defaultKind?: CredentialKind;
}

/** 新建或修改一条记录。只记信息，服务端会拒绝像密钥本身的内容。 */
export default function CredentialDialog({
  open,
  onClose,
  credential: c,
  defaultKind,
}: Props) {
  const t = useT();
  const create = useCreateCredential();
  const update = useUpdateCredential();
  const [kind, setKind] = useState<CredentialKind>("api_key");
  const [name, setName] = useState("");
  const [platform, setPlatform] = useState("");
  const [account, setAccount] = useState("");
  const [usedBy, setUsedBy] = useState("");
  const [scopes, setScopes] = useState("");
  const [hint, setHint] = useState("");
  const [createdOn, setCreatedOn] = useState("");
  const [rotatedOn, setRotatedOn] = useState("");
  const [expiresOn, setExpiresOn] = useState("");
  const [rotateEvery, setRotateEvery] = useState("");
  const [remind, setRemind] = useState("30, 7");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setKind(c?.kind ?? defaultKind ?? "api_key");
    setName(c?.name ?? "");
    setPlatform(c?.platform ?? "");
    setAccount(c?.account ?? "");
    setUsedBy(c?.usedBy.join("\n") ?? "");
    setScopes(c?.scopes ?? "");
    setHint(c?.hint ?? "");
    setCreatedOn(c?.createdOn ?? "");
    setRotatedOn(c?.rotatedOn ?? "");
    setExpiresOn(c?.expiresOn ?? "");
    setRotateEvery(c?.rotateEveryDays ? String(c.rotateEveryDays) : "");
    setRemind(c ? formatDays(c.remindDays) : "30, 7");
    setNotes(c?.notes ?? "");
  }, [open, c, defaultKind]);

  const saving = create.isPending || update.isPending;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return setError(t("Please enter a name"));
    const days = parseDays(remind);
    if (!days)
      return setError(
        t("Remind days: whole numbers from 1 to 3650, at most 8"),
      );
    const every = rotateEvery.trim() === "" ? 0 : Number(rotateEvery);
    if (!Number.isInteger(every) || every < 0 || every > 3650)
      return setError(t("Rotation period: whole days from 0 to 3650"));
    if (every > 0 && !createdOn && !rotatedOn)
      return setError(
        t("A rotation period needs the created date or the last rotation date"),
      );
    const body = {
      kind,
      name: name.trim(),
      platform: platform.trim(),
      account: account.trim(),
      usedBy: parseLines(usedBy),
      scopes: scopes.trim(),
      hint: hint.trim(),
      createdOn,
      rotatedOn,
      expiresOn,
      rotateEveryDays: every,
      remindDays: days,
      notes,
    };
    try {
      if (c) await update.mutateAsync({ id: c.id, body });
      else await create.mutateAsync(body);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={c ? t("Edit credential") : t("New credential")}
      wide
    >
      <form onSubmit={submit}>
        <p className="credentials-warning">
          {t("Do not enter the key itself. Only record facts about it.")}
        </p>
        <div className="credentials-form-row">
          <label className="xc-field credentials-narrow">
            <span>{t("Type")}</span>
            <select
              className="xc-select"
              value={kind}
              onChange={(e) => setKind(e.target.value as CredentialKind)}
            >
              {KINDS.map((k) => (
                <option key={k} value={k}>
                  {t(KIND_LABELS[k])}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              autoFocus
              placeholder="例如 部署用的 GitHub 令牌"
            />
          </label>
        </div>
        <div className="credentials-form-row">
          <label className="xc-field">
            <span>{t("Platform")}</span>
            <input
              className="xc-input"
              value={platform}
              onChange={(e) => setPlatform(e.target.value)}
              maxLength={100}
              placeholder="GitHub、OpenAI"
            />
          </label>
          <label className="xc-field">
            <span>{t("Account")}</span>
            <input
              className="xc-input"
              value={account}
              onChange={(e) => setAccount(e.target.value)}
              maxLength={200}
            />
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Used on")}</span>
          <textarea
            className="xc-input"
            rows={3}
            value={usedBy}
            onChange={(e) => setUsedBy(e.target.value)}
          />
          <small>
            {t(
              "One server or project per line. When a key leaks, search for it here.",
            )}
          </small>
        </label>
        <div className="credentials-form-row">
          <label className="xc-field">
            <span>{t("Permissions")}</span>
            <input
              className="xc-input"
              value={scopes}
              onChange={(e) => setScopes(e.target.value)}
              maxLength={200}
              placeholder="repo, workflow"
            />
          </label>
          <label className="xc-field credentials-narrow">
            <span>{t("Last characters")}</span>
            <input
              className="xc-input"
              value={hint}
              onChange={(e) => setHint(e.target.value)}
              maxLength={16}
              autoComplete="off"
            />
            <small>{t("To recognise it on the platform page.")}</small>
          </label>
        </div>
        <div className="credentials-form-row">
          <label className="xc-field">
            <span>{t("Created on")}</span>
            <input
              className="xc-input"
              type="date"
              value={createdOn}
              onChange={(e) => setCreatedOn(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Last rotated on")}</span>
            <input
              className="xc-input"
              type="date"
              value={rotatedOn}
              onChange={(e) => setRotatedOn(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Expires on")}</span>
            <input
              className="xc-input"
              type="date"
              value={expiresOn}
              onChange={(e) => setExpiresOn(e.target.value)}
            />
            <small>{t("Leave empty if it never expires.")}</small>
          </label>
        </div>
        <div className="credentials-form-row">
          <label className="xc-field">
            <span>{t("Rotate every (days)")}</span>
            <input
              className="xc-input"
              value={rotateEvery}
              onChange={(e) => setRotateEvery(e.target.value)}
              inputMode="numeric"
              placeholder="90"
            />
            <small>
              {t("You are reminded when it is not rotated in time.")}
            </small>
          </label>
          <label className="xc-field">
            <span>{t("Remind before expiry (days)")}</span>
            <input
              className="xc-input"
              value={remind}
              onChange={(e) => setRemind(e.target.value)}
              inputMode="numeric"
            />
            <small>
              {t("For example 30, 7. You are also told on the day it expires.")}
            </small>
          </label>
        </div>
        <div className="xc-field">
          <span>{t("Credential remarks")}</span>
          <MarkdownEditor
            label={t("Credential notes")}
            value={notes}
            onChange={setNotes}
            minRows={3}
          />
        </div>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button type="submit" className="xc-btn primary" disabled={saving}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
