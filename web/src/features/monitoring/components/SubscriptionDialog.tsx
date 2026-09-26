import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useSaveSubscription,
  type Subscription,
  type SubscriptionCategory,
  type SubscriptionCycle,
} from "../api";
import { parseRemindDays } from "../lib";

export const categoryLabels: Record<SubscriptionCategory, string> = {
  server: "Server",
  domain: "Domain",
  saas: "Software",
  other: "Other",
};

export const cycleLabels: Record<SubscriptionCycle, string> = {
  monthly: "Monthly",
  yearly: "Yearly",
  custom_days: "Every N days",
};

function todayPlus(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

interface Props {
  open: boolean;
  onClose: () => void;
  subscription?: Subscription | null;
}

export default function SubscriptionDialog({ open, onClose, subscription: sub }: Props) {
  const t = useT();
  const save = useSaveSubscription();
  const [name, setName] = useState("");
  const [category, setCategory] = useState<SubscriptionCategory>("other");
  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState("CNY");
  const [cycle, setCycle] = useState<SubscriptionCycle>("monthly");
  const [cycleDays, setCycleDays] = useState("30");
  const [next, setNext] = useState("");
  const [remind, setRemind] = useState("7, 1");
  const [autoRenew, setAutoRenew] = useState(false);
  const [url, setUrl] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setName(sub?.name ?? "");
    setCategory(sub?.category ?? "other");
    setAmount(sub ? String(sub.amount) : "");
    setCurrency(sub?.currency ?? "CNY");
    setCycle(sub?.cycle ?? "monthly");
    setCycleDays(String(sub?.cycleDays || 30));
    setNext(sub?.nextRenewal ?? todayPlus(30));
    setRemind(sub ? sub.remindDaysBefore.join(", ") : "7, 1");
    setAutoRenew(sub?.autoRenew ?? false);
    setUrl(sub?.url ?? "");
    setNote(sub?.note ?? "");
  }, [open, sub]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const value = Number(amount);
    const days = parseRemindDays(remind);
    if (!name.trim()) return setError(t("Please enter a name"));
    if (amount.trim() === "" || !Number.isFinite(value) || value < 0) return setError(t("Please enter the amount"));
    if (!/^\d{4}-\d{2}-\d{2}$/.test(next)) return setError(t("Please pick the next renewal date"));
    if (days === null) return setError(t("Reminder days look wrong, for example 7, 1"));
    const fields = {
      name: name.trim(),
      category,
      amount: value,
      currency: currency.trim().toUpperCase() || "CNY",
      cycle,
      cycleDays: cycle === "custom_days" ? Number(cycleDays) || 30 : undefined,
      nextRenewal: next,
      remindDaysBefore: days,
      autoRenew,
      url: url.trim(),
      note,
    };
    try {
      if (sub) await save.mutateAsync({ id: sub.id, patch: fields });
      else await save.mutateAsync({ create: fields });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog open={open} onClose={onClose} title={sub ? t("Edit subscription") : t("New subscription")}>
      <form onSubmit={submit}>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input className="xc-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={100} autoFocus placeholder="例如 阿里云 ECS" />
          </label>
          <label className="xc-field monitoring-narrow-field">
            <span>{t("Category")}</span>
            <select className="xc-select" value={category} onChange={(e) => setCategory(e.target.value as SubscriptionCategory)}>
              {(Object.keys(categoryLabels) as SubscriptionCategory[]).map((c) => (
                <option key={c} value={c}>
                  {t(categoryLabels[c])}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Amount")}</span>
            <input className="xc-input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="99" />
          </label>
          <label className="xc-field monitoring-narrow-field">
            <span>{t("Currency")}</span>
            <input className="xc-input" value={currency} onChange={(e) => setCurrency(e.target.value)} maxLength={8} list="monitoring-currencies" />
            <datalist id="monitoring-currencies">
              {["CNY", "USD", "EUR", "HKD", "JPY", "GBP"].map((c) => (
                <option key={c} value={c} />
              ))}
            </datalist>
          </label>
        </div>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Billing cycle")}</span>
            <select className="xc-select" value={cycle} onChange={(e) => setCycle(e.target.value as SubscriptionCycle)}>
              {(Object.keys(cycleLabels) as SubscriptionCycle[]).map((c) => (
                <option key={c} value={c}>
                  {t(cycleLabels[c])}
                </option>
              ))}
            </select>
          </label>
          {cycle === "custom_days" && (
            <label className="xc-field monitoring-narrow-field">
              <span>{t("Days")}</span>
              <input className="xc-input" inputMode="numeric" value={cycleDays} onChange={(e) => setCycleDays(e.target.value.replace(/\D/g, ""))} />
            </label>
          )}
        </div>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Next renewal")}</span>
            <input className="xc-input" type="date" value={next} onChange={(e) => setNext(e.target.value)} />
          </label>
          <label className="xc-field">
            <span>{t("Remind days before")}</span>
            <input className="xc-input" value={remind} onChange={(e) => setRemind(e.target.value)} placeholder="7, 1" />
          </label>
        </div>
        <label className="monitoring-check">
          <input type="checkbox" checked={autoRenew} onChange={(e) => setAutoRenew(e.target.checked)} />
          <span>{t("Renews automatically")}</span>
          <small className="xc-muted">{t("The date moves to the next cycle by itself.")}</small>
        </label>
        <label className="xc-field">
          <span>{t("Link")}</span>
          <input className="xc-input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={t("optional")} />
        </label>
        <label className="xc-field">
          <span>{t("Remark")}</span>
          <textarea className="xc-textarea" value={note} onChange={(e) => setNote(e.target.value)} rows={2} />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button type="submit" className="xc-btn primary" disabled={save.isPending}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
