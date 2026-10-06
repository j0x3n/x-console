import { useState } from "react";
import { Plus, Trash2, VolumeX } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useMailAccounts } from "../mail/api";
import {
  useCreateNotifyMute,
  useDeleteNotifyMute,
  useNotifyMutes,
  type NotifyMute,
} from "./api";
import {
  CUSTOM_KIND,
  KIND_PRESETS,
  kindLabel,
  mailScope,
  mailScopeId,
  validKindPattern,
} from "./mutes";
import { useNotifyTargetOptions } from "./NotifyTargets";
import "./i18n";

const onError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 设置 → 通知里的“静音规则”（B113）：某类通知不发到某个设备或渠道。通知铃不受影响。 */
export default function MutesCard() {
  const t = useT();
  const mutes = useNotifyMutes();
  const remove = useDeleteNotifyMute();
  const mail = useMailAccounts();
  const { options } = useNotifyTargetOptions();
  const [adding, setAdding] = useState(false);

  const scopeText = (m: NotifyMute) => {
    if (!m.scope) return t("All");
    const id = mailScopeId(m.scope);
    if (id === null) return m.scope;
    const acc = mail.data?.find((a) => a.id === id);
    return acc ? acc.name : t("Deleted mailbox");
  };
  const targetText = (target: string) => {
    const hit = options.find((o) => o.id === target);
    if (hit) return hit.label;
    if (target === "webpush") return t("Browser push");
    return target.startsWith("webpush:") ? t("Removed device") : target;
  };
  const kindText = (pattern: string) => {
    const label = kindLabel(pattern);
    return label ? t(label) : pattern;
  };

  return (
    <section className="xc-card notify-mutes">
      <div className="xc-card-head">
        <h2>{t("Mute rules")}</h2>
        <button className="xc-btn small" onClick={() => setAdding(true)}>
          <Plus size={13} /> {t("Add mute rule")}
        </button>
      </div>
      <p className="xc-muted notify-help">
        {t(
          "A muted kind of notification is not sent to that device or channel. The notification bell always has it. For one mailbox, use its settings under Mail.",
        )}
      </p>
      {mutes.isPending ? (
        <Loading />
      ) : mutes.isError ? (
        <ErrorState error={mutes.error} onRetry={() => mutes.refetch()} />
      ) : mutes.data.length === 0 ? (
        <p className="xc-muted">{t("No mute rules")}</p>
      ) : (
        <ul className="notify-mute-list">
          {mutes.data.map((m) => (
            <li key={m.id}>
              <VolumeX size={14} />
              <span className="notify-mute-main">
                <strong>{kindText(m.kindPattern)}</strong>
                <small>
                  {scopeText(m)} → {targetText(m.target)}
                </small>
              </span>
              <button
                className="xc-btn small ghost danger"
                aria-label={`${t("Delete")} ${kindText(m.kindPattern)}`}
                title={t("Delete")}
                onClick={() =>
                  remove.mutate(m.id, {
                    onSuccess: () => toast(t("Deleted")),
                    onError,
                  })
                }
              >
                <Trash2 size={14} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <AddMuteDialog open={adding} onClose={() => setAdding(false)} />
    </section>
  );
}

function AddMuteDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const create = useCreateNotifyMute();
  const mail = useMailAccounts();
  const { options } = useNotifyTargetOptions();
  const [preset, setPreset] = useState("mail.*");
  const [custom, setCustom] = useState("");
  const [scope, setScope] = useState("");
  const [target, setTarget] = useState("");

  const pattern = preset === CUSTOM_KIND ? custom.trim() : preset;
  const isMail = pattern.startsWith("mail.");
  const valid = validKindPattern(pattern) && target !== "";

  const submit = () => {
    create.mutate(
      { kindPattern: pattern, ...(isMail && scope ? { scope } : {}), target },
      {
        onSuccess: () => {
          toast(t("Saved"));
          onClose();
        },
        onError,
      },
    );
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Add mute rule")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={!valid || create.isPending}
            onClick={submit}
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <div className="notify-mute-form">
        <div className="xc-field">
          <label htmlFor="mute-kind">{t("Kind")}</label>
          <select
            id="mute-kind"
            className="xc-select"
            value={preset}
            onChange={(e) => {
              setPreset(e.target.value);
              setScope("");
            }}
          >
            {KIND_PRESETS.map((p) => (
              <option key={p.pattern} value={p.pattern}>
                {t(p.label)}
              </option>
            ))}
            <option value={CUSTOM_KIND}>{t("Custom")}</option>
          </select>
          {preset === CUSTOM_KIND && (
            <input
              className="xc-input xc-mono"
              aria-label={t("Custom kind")}
              value={custom}
              placeholder="host.alert*"
              onChange={(e) => setCustom(e.target.value)}
            />
          )}
        </div>
        {isMail && (
          <div className="xc-field">
            <label htmlFor="mute-scope">{t("Mailbox")}</label>
            <select
              id="mute-scope"
              className="xc-select"
              value={scope}
              onChange={(e) => setScope(e.target.value)}
            >
              <option value="">{t("All mailboxes")}</option>
              {(mail.data ?? []).map((a) => (
                <option key={a.id} value={mailScope(a.id)}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <div className="xc-field">
          <label htmlFor="mute-target">{t("Do not send to")}</label>
          <select
            id="mute-target"
            className="xc-select"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            <option value="">{t("Choose a device or channel")}</option>
            {options.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label}
                {o.mine ? ` (${t("This device")})` : ""}
              </option>
            ))}
          </select>
        </div>
      </div>
    </Dialog>
  );
}
