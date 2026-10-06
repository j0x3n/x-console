import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  usePutQuotaNotify,
  useQuotaNotify,
  type QuotaNotifySettings,
} from "./api";

const ROWS: Array<{
  key: keyof QuotaNotifySettings;
  label: string;
  hint: string;
}> = [
  {
    key: "low",
    label: "Running out",
    hint: "When a window has 10% or less left.",
  },
  {
    key: "empty",
    label: "Used up",
    hint: "When a window has nothing left.",
  },
  {
    key: "balance",
    label: "Low DeepSeek balance",
    hint: "When a balance falls under the limit set on the account.",
  },
  {
    key: "failed",
    label: "Reading keeps failing",
    hint: "After 3 failed readings in a row (2 for Claude).",
  },
];

/** 额度通知的开关（B112）。每个窗口每个周期只通知一次，重置后再通知。 */
export default function NotifyDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useQuotaNotify(open);
  const save = usePutQuotaNotify();
  const toggle = (key: keyof QuotaNotifySettings, value: boolean) => {
    if (!settings.data) return;
    save.mutate(
      { ...settings.data, [key]: value },
      { onError: (e) => toast({ message: errorMessage(e), tone: "error" }) },
    );
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Quota notifications")}
      description={t(
        "Each window is told once per period. It is told again after the window resets.",
      )}
      footer={
        <button className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      }
    >
      <div className="quota-form">
        {ROWS.map((r) => (
          <label key={r.key} className="xc-check">
            <input
              type="checkbox"
              checked={settings.data?.[r.key] ?? true}
              disabled={!settings.data || save.isPending}
              onChange={(e) => toggle(r.key, e.target.checked)}
            />
            {t(r.label)}
            <small className="xc-check-hint">{t(r.hint)}</small>
          </label>
        ))}
      </div>
    </Dialog>
  );
}
