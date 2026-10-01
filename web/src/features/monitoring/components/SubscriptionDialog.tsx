import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useSaveSubscription,
  useSubscriptionCategories,
  type CycleUnit,
  type Subscription,
  type SubscriptionCategory,
  type SubscriptionCategoryItem,
  type SubscriptionCycle,
} from "../api";
import { cycleOf, legacyCycle, unitLabels } from "../lib";
import CategoryManager from "./CategoryManager";

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

/** 常用币种（B23）。其他的选“其他”后手动填。 */
export const currencies = ["CNY", "USD", "EUR", "HKD", "JPY", "GBP", "SGD"];
const OTHER = "__other";
const MANAGE = "__manage";
const units: CycleUnit[] = ["minute", "hour", "day", "week", "month", "year"];
/** 提前提醒的选项（B49 加了 3 天）。新订阅默认全选。 */
const remindChoices = [1, 3, 7];
const defaultRemind = [7, 3, 1];

/** 订阅显示用的分类名：新接口有 categoryName，旧接口用固定的四个。 */
export function categoryName(
  sub: Pick<Subscription, "category" | "categoryName">,
  t: (key: string) => string,
): string {
  return sub.categoryName || t(categoryLabels[sub.category]);
}

function todayPlus(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 旧接口要 cycleDays 时的近似天数。 */
function approxDays(count: number, unit: CycleUnit): number {
  const perUnit = { minute: 1, hour: 1, day: 1, week: 7, month: 30, year: 365 };
  return Math.min(3660, Math.max(1, count * perUnit[unit]));
}

interface Props {
  open: boolean;
  onClose: () => void;
  subscription?: Subscription | null;
}

export default function SubscriptionDialog({
  open,
  onClose,
  subscription: sub,
}: Props) {
  const t = useT();
  const save = useSaveSubscription();
  const categories = useSubscriptionCategories();
  // 分类接口上线了，说明后端也认识新的周期写法。
  const live = categories.isSuccess;
  const [name, setName] = useState("");
  const [account, setAccount] = useState("");
  const [category, setCategory] = useState<string>("other");
  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState("CNY");
  const [customCurrency, setCustomCurrency] = useState("");
  const [count, setCount] = useState("1");
  const [unit, setUnit] = useState<CycleUnit>("month");
  const [next, setNext] = useState("");
  const [remind, setRemind] = useState<number[]>(defaultRemind);
  const [autoRenew, setAutoRenew] = useState(false);
  const [url, setUrl] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [managing, setManaging] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError("");
    setName(sub?.name ?? "");
    setAccount(sub?.account ?? "");
    setCategory(
      sub?.categoryId ? String(sub.categoryId) : (sub?.category ?? "other"),
    );
    setAmount(sub ? String(sub.amount) : "");
    const cur = sub?.currency ?? "CNY";
    setCurrency(currencies.includes(cur) ? cur : OTHER);
    setCustomCurrency(currencies.includes(cur) ? "" : cur);
    const c = sub ? cycleOf(sub) : { count: 1, unit: "month" as const };
    setCount(String(c.count));
    setUnit(c.unit);
    setNext(sub?.nextRenewal ?? todayPlus(30));
    setRemind(sub ? sub.remindDaysBefore : defaultRemind);
    setAutoRenew(sub?.autoRenew ?? false);
    setUrl(sub?.url ?? "");
    setNote(sub?.note ?? "");
  }, [open, sub]);

  // 新接口：分类的值是 id。旧分类（server 这种）换成对应的 id。
  const items: SubscriptionCategoryItem[] = categories.data ?? [];
  useEffect(() => {
    if (!live || /^\d+$/.test(category)) return;
    const match = items.find((c) => c.builtin === category);
    if (match) setCategory(String(match.id));
  }, [live, items, category]);

  const toggleRemind = (day: number) =>
    setRemind((prev) =>
      prev.includes(day)
        ? prev.filter((d) => d !== day)
        : [...prev, day].sort((a, b) => b - a),
    );

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const value = Number(amount);
    const n = Number(count);
    if (!name.trim()) return setError(t("Please enter a name"));
    if (amount.trim() === "" || !Number.isFinite(value) || value < 0)
      return setError(t("Please enter the amount"));
    if (!Number.isInteger(n) || n < 1 || n > 1000)
      return setError(t("The cycle should be a whole number from 1 to 1000"));
    if (!/^\d{4}-\d{2}-\d{2}$/.test(next))
      return setError(t("Please pick the next renewal date"));
    const code =
      currency === OTHER ? customCurrency.trim().toUpperCase() : currency;
    if (!/^[A-Z]{3}$/.test(code))
      return setError(t("Enter a three-letter currency code, like THB"));
    const legacy = legacyCycle({ count: n, unit });
    if (!live && !legacy)
      return setError(
        t(
          "Until the server is updated, only days, weeks, one month or one year can be saved.",
        ),
      );
    const byId = /^\d+$/.test(category)
      ? items.find((c) => String(c.id) === category)
      : undefined;
    const fields = {
      name: name.trim(),
      account: account.trim(),
      category: (byId
        ? (byId.builtin ?? "other")
        : category) as SubscriptionCategory,
      ...(byId ? { categoryId: byId.id } : {}),
      amount: value,
      currency: code,
      cycle: legacy?.cycle ?? ("custom_days" as const),
      cycleDays: legacy ? legacy.cycleDays : approxDays(n, unit),
      ...(live ? { cycleCount: n, cycleUnit: unit } : {}),
      nextRenewal: next,
      remindDaysBefore: remind,
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

  const extraRemind = remind.filter((d) => !remindChoices.includes(d));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={sub ? t("Edit subscription") : t("New subscription")}
    >
      <form onSubmit={submit}>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              autoFocus
              placeholder="例如 阿里云 ECS"
            />
          </label>
          <label className="xc-field monitoring-narrow-field">
            <span>{t("Category")}</span>
            <select
              className="xc-select"
              value={category}
              onChange={(e) =>
                e.target.value === MANAGE
                  ? setManaging(true)
                  : setCategory(e.target.value)
              }
            >
              {live
                ? items.map((c) => (
                    <option key={c.id} value={String(c.id)}>
                      {c.builtin ? t(categoryLabels[c.builtin]) : c.name}
                    </option>
                  ))
                : (Object.keys(categoryLabels) as SubscriptionCategory[]).map(
                    (c) => (
                      <option key={c} value={c}>
                        {t(categoryLabels[c])}
                      </option>
                    ),
                  )}
              {live && (
                <option value={MANAGE}>{t("Manage categories…")}</option>
              )}
            </select>
          </label>
        </div>
        <label className="xc-field">
          <span>{t("Account")}</span>
          <input
            className="xc-input"
            value={account}
            onChange={(e) => setAccount(e.target.value)}
            maxLength={200}
            autoComplete="off"
            placeholder={t("Login email or username. Optional.")}
          />
        </label>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Amount")}</span>
            <input
              className="xc-input"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="99"
            />
          </label>
          <div className="xc-field monitoring-narrow-field">
            <span>{t("Currency")}</span>
            <div className="monitoring-currency">
              <select
                className="xc-select"
                aria-label={t("Currency")}
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              >
                {currencies.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
                <option value={OTHER}>{t("Other…")}</option>
              </select>
              {currency === OTHER && (
                <input
                  className="xc-input"
                  aria-label={t("Currency code")}
                  value={customCurrency}
                  onChange={(e) => setCustomCurrency(e.target.value)}
                  maxLength={3}
                  placeholder="THB"
                  autoFocus
                />
              )}
            </div>
          </div>
        </div>
        <div className="xc-field">
          <span>{t("Billing cycle")}</span>
          <div className="monitoring-cycle">
            <span className="monitoring-cycle-every">{t("Every")}</span>
            <input
              className="xc-input"
              inputMode="numeric"
              aria-label={t("How many")}
              value={count}
              onChange={(e) => setCount(e.target.value.replace(/\D/g, ""))}
            />
            <select
              className="xc-select"
              aria-label={t("Unit")}
              value={unit}
              onChange={(e) => setUnit(e.target.value as CycleUnit)}
            >
              {units.map((u) => (
                <option key={u} value={u}>
                  {t(unitLabels[u])}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="monitoring-form-row">
          <label className="xc-field">
            <span>{t("Next renewal")}</span>
            <input
              className="xc-input"
              type="date"
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </label>
          <div className="xc-field">
            <span>{t("Remind before renewal")}</span>
            <div className="monitoring-remind">
              {[...remindChoices, ...extraRemind]
                .sort((a, b) => a - b)
                .map((day) => (
                  <label key={day} className="xc-check">
                    <input
                      type="checkbox"
                      checked={remind.includes(day)}
                      onChange={() => toggleRemind(day)}
                    />
                    <span>
                      {t("Ahead by")} {day} {t("days")}
                    </span>
                  </label>
                ))}
            </div>
          </div>
        </div>
        <label className="monitoring-check">
          <input
            type="checkbox"
            checked={autoRenew}
            onChange={(e) => setAutoRenew(e.target.checked)}
          />
          <span>{t("Renews automatically")}</span>
          <small className="xc-muted">
            {t("The date moves to the next cycle by itself.")}
          </small>
        </label>
        <label className="xc-field">
          <span>{t("Link")}</span>
          <input
            className="xc-input"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t("optional")}
          />
        </label>
        <label className="xc-field">
          <span>{t("Remark")}</span>
          <textarea
            className="xc-textarea"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
          />
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={save.isPending}
          >
            {t("Save")}
          </button>
        </div>
      </form>
      {managing && (
        <CategoryManager
          categories={items}
          onClose={() => setManaging(false)}
        />
      )}
    </Dialog>
  );
}
