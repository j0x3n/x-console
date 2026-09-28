import { useState } from "react";
import {
  Archive,
  ArchiveRestore,
  CalendarCheck,
  ExternalLink,
  Pencil,
  Plus,
  Receipt,
  Trash2,
} from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useDeleteSubscription,
  useSaveSubscription,
  useSubscriptionEvents,
  useSubscriptions,
  useSubscriptionSummary,
  type Subscription,
} from "../api";
import {
  formatMoney,
  nextRenewal,
  renewalTone,
  sortSubscriptions,
} from "../lib";
import {
  longDate,
  shortDateTime,
  showError,
  useIdParam,
  useParam,
} from "./common";
import SubscriptionDialog, {
  categoryLabels,
  cycleLabels,
} from "./SubscriptionDialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";

/** 订阅与续费：支出汇总、列表、详情。 */
export default function SubscriptionsTab() {
  const t = useT();
  const [archived, setArchived] = useState(false);
  const list = useSubscriptions(archived);
  const [creating, setCreating] = useParam("new");
  const [openId, setOpenId] = useIdParam("subscription");
  const items = sortSubscriptions(list.data ?? []);
  const open = items.find((s) => s.id === openId) ?? null;

  return (
    <>
      {!archived && <SpendSummary />}
      <div className="monitoring-toolbar">
        <label className="monitoring-check">
          <input
            type="checkbox"
            checked={archived}
            onChange={(e) => setArchived(e.target.checked)}
          />
          <span>{t("Show archived")}</span>
        </label>
      </div>
      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          title={archived ? t("Nothing archived") : t("No subscriptions yet")}
          icon={<Receipt size={28} />}
        >
          {!archived && (
            <>
              <span>
                {t(
                  "Add servers, domains and apps you pay for. You get a reminder before each renewal.",
                )}
              </span>
              <button
                className="xc-btn primary small"
                onClick={() => setCreating("1")}
              >
                <Plus size={14} /> {t("New subscription")}
              </button>
            </>
          )}
        </EmptyState>
      ) : (
        <div className="xc-card monitoring-list">
          {items.map((s) => (
            <SubscriptionRow
              key={s.id}
              sub={s}
              onOpen={() => setOpenId(s.id)}
            />
          ))}
        </div>
      )}
      <SubscriptionDialog
        open={creating === "1"}
        onClose={() => setCreating(null)}
      />
      <SubscriptionDetail sub={open} onClose={() => setOpenId(null)} />
    </>
  );
}

function SpendSummary() {
  const t = useT();
  const summary = useSubscriptionSummary();
  if (!summary.data || summary.data.totals.length === 0) return null;
  return (
    <div className="monitoring-summary">
      {summary.data.totals.map((x) => (
        <div key={x.currency} className="xc-card monitoring-stat">
          <small>
            {t("Per month")} · {x.currency}
          </small>
          <strong>{formatMoney(x.monthly, x.currency)}</strong>
          <small className="xc-muted">
            {t("Per year")} {formatMoney(x.yearly, x.currency)} · {x.count}{" "}
            {t("items")}
          </small>
        </div>
      ))}
    </div>
  );
}

function daysText(t: (s: string) => string, days: number) {
  if (days < 0) return `${t("Overdue")} ${-days} ${t("days")}`;
  if (days === 0) return t("Today");
  return `${days} ${t("days to renewal")}`;
}

function SubscriptionRow({
  sub: s,
  onOpen,
}: {
  sub: Subscription;
  onOpen: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  return (
    <button className="monitoring-row" onClick={onOpen}>
      <span className="monitoring-row-main">
        <strong>
          {s.name}{" "}
          <span className="xc-badge">{t(categoryLabels[s.category])}</span>
        </strong>
        <small>
          {formatMoney(s.amount, s.currency)} ·{" "}
          {s.cycle === "custom_days"
            ? `${s.cycleDays} ${t("days")}`
            : t(cycleLabels[s.cycle])}
          {s.autoRenew && ` · ${t("auto renew")}`}
        </small>
      </span>
      <span className="monitoring-row-side">
        {s.archivedAt ? (
          <span className="xc-badge">{t("Archived")}</span>
        ) : (
          <span className={`xc-badge ${renewalTone(s.daysLeft)}`}>
            {daysText(t, s.daysLeft)}
          </span>
        )}
        <small className="xc-muted">{longDate(s.nextRenewal, language)}</small>
      </span>
    </button>
  );
}

const eventLabels: Record<string, string> = {
  created: "Added",
  reminded: "Reminded",
  renewed: "Renewed",
  updated: "Date changed",
};

function SubscriptionDetail({
  sub,
  onClose,
}: {
  sub: Subscription | null;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const events = useSubscriptionEvents(sub?.id ?? null);
  const save = useSaveSubscription();
  const remove = useDeleteSubscription();
  const [editing, setEditing] = useState(false);
  if (!sub) return null;

  const markRenewed = () =>
    save.mutate(
      {
        id: sub.id,
        patch: {
          nextRenewal: nextRenewal(sub.nextRenewal, sub.cycle, sub.cycleDays),
        },
      },
      {
        onSuccess: () => toast(t("Moved to the next cycle")),
        onError: showError,
      },
    );
  const archive = () =>
    save.mutate(
      { id: sub.id, patch: { archived: !sub.archivedAt } },
      {
        onSuccess: () => {
          toast(sub.archivedAt ? t("Unarchived") : t("Archived"));
          onClose();
        },
        onError: showError,
      },
    );

  return (
    <>
      <Dialog open={!editing} onClose={onClose} title={sub.name} wide>
        <div className="monitoring-facts">
          <div>
            <small>{t("Amount")}</small>
            <strong>{formatMoney(sub.amount, sub.currency)}</strong>
          </div>
          <div>
            <small>{t("Billing cycle")}</small>
            <strong>
              {sub.cycle === "custom_days"
                ? `${sub.cycleDays} ${t("days")}`
                : t(cycleLabels[sub.cycle])}
            </strong>
          </div>
          <div>
            <small>{t("Next renewal")}</small>
            <strong>{longDate(sub.nextRenewal, language)}</strong>
          </div>
          <div>
            <small>{t("Per month")}</small>
            <strong>{formatMoney(sub.monthlyCost, sub.currency)}</strong>
          </div>
          <div>
            <small>{t("Reminders")}</small>
            <strong>
              {sub.remindDaysBefore.length
                ? sub.remindDaysBefore.map((d) => `${d}`).join(", ") +
                  ` ${t("days before")}`
                : t("Off")}
            </strong>
          </div>
        </div>
        {(sub.url || sub.note) && (
          <div className="monitoring-note">
            {sub.url && (
              <a href={sub.url} target="_blank" rel="noreferrer">
                <ExternalLink size={13} /> {sub.url}
              </a>
            )}
            {sub.note && <p>{sub.note}</p>}
          </div>
        )}
        <div className="monitoring-detail-bar">
          <strong>{t("History")}</strong>
        </div>
        {events.isPending ? (
          <Loading />
        ) : events.isError ? (
          <ErrorState error={events.error} onRetry={() => events.refetch()} />
        ) : (
          <ul className="monitoring-events">
            {events.data.map((e) => (
              <li key={e.id}>
                <span className="xc-badge">
                  {t(eventLabels[e.kind] ?? e.kind)}
                </span>
                <span>{e.detail}</span>
                <small className="xc-muted">
                  {shortDateTime(e.at, language)}
                </small>
              </li>
            ))}
          </ul>
        )}
        <div className="xc-dialog-actions monitoring-actions">
          <button
            className="xc-btn ghost danger"
            disabled={remove.isPending}
            onClick={async () =>
              (await confirmAction({
                title: `${t("Delete")}“${sub.name}”？`,
              })) &&
              remove.mutate(sub.id, {
                onSuccess: () => {
                  toast(t("Deleted"));
                  onClose();
                },
                onError: showError,
              })
            }
          >
            <Trash2 size={14} /> {t("Delete")}
          </button>
          <button
            className="xc-btn ghost"
            disabled={save.isPending}
            onClick={archive}
          >
            {sub.archivedAt ? (
              <ArchiveRestore size={14} />
            ) : (
              <Archive size={14} />
            )}{" "}
            {sub.archivedAt ? t("Unarchive") : t("Archive")}
          </button>
          <span className="xc-spacer" />
          <button className="xc-btn" onClick={() => setEditing(true)}>
            <Pencil size={14} /> {t("Edit")}
          </button>
          {!sub.autoRenew && !sub.archivedAt && (
            <button
              className="xc-btn primary"
              disabled={save.isPending}
              onClick={markRenewed}
            >
              <CalendarCheck size={14} /> {t("Mark renewed")}
            </button>
          )}
        </div>
      </Dialog>
      <SubscriptionDialog
        open={editing}
        onClose={() => setEditing(false)}
        subscription={sub}
      />
    </>
  );
}
