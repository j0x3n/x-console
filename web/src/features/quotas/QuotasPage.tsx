import { useState } from "react";
import { Bell, Gauge, Plus, RefreshCw } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import AccountCard from "./AccountCard";
import AccountDialog from "./AccountDialog";
import NotifyDialog from "./NotifyDialog";
import {
  useDeleteQuotaAccount,
  useQuotaAccounts,
  useRefreshQuotaAccount,
  useReorderQuotaAccounts,
  type QuotaAccount,
} from "./api";
import {
  KIND_NAMES,
  durationText,
  groupByKind,
  moveWithinKind,
  nextReset,
  tightestWindow,
} from "./format";
import { useNow } from "./useNow";
import "./i18n";
import "./quotas.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** AI 额度（B111）：Claude、Codex、Grok 的额度窗口和重置时间，DeepSeek 的余额。 */
export default function QuotasPage() {
  const t = useT();
  const language = useLanguage();
  const now = useNow();
  const list = useQuotaAccounts();
  const refresh = useRefreshQuotaAccount();
  const remove = useDeleteQuotaAccount();
  const reorder = useReorderQuotaAccounts();
  const [dialog, setDialog] = useState<{ account?: QuotaAccount } | "closed">(
    "closed",
  );
  const [notifyOpen, setNotifyOpen] = useState(false);
  const [refreshingId, setRefreshingId] = useState<number | null>(null);
  const [refreshingAll, setRefreshingAll] = useState(false);

  const accounts = list.data ?? [];
  const failed = accounts.filter((a) => a.status === "error").length;

  const refreshOne = (id: number) => {
    setRefreshingId(id);
    refresh.mutate(id, {
      onError: fail,
      onSettled: () => setRefreshingId(null),
    });
  };
  const refreshAll = async () => {
    setRefreshingAll(true);
    try {
      await Promise.all(accounts.map((a) => refresh.mutateAsync(a.id)));
    } catch (e) {
      fail(e);
    } finally {
      setRefreshingAll(false);
    }
  };
  const onDelete = async (a: QuotaAccount) => {
    const ok = await confirmAction({
      title: `${t("Delete account")} ${a.name}？`,
      description: t(
        "This only removes the record here. The sign-in on the machine is not touched.",
      ),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(a.id, {
      onSuccess: () => toast(t("Deleted")),
      onError: fail,
    });
  };
  const move = (id: number, delta: -1 | 1) => {
    const ids = moveWithinKind(accounts, id, delta);
    if (ids) reorder.mutate(ids, { onError: fail });
  };

  const addButton = (
    <button
      className="xc-btn small primary"
      title={t("Add account")}
      aria-label={t("Add account")}
      onClick={() => setDialog({})}
    >
      <Plus size={14} /> {t("Add account")}
    </button>
  );

  let content;
  if (list.isPending) content = <Loading />;
  else if (list.isError && isNotLive(list.error))
    content = <NotLive name={t("AI quotas")} icon={<Gauge size={28} />} />;
  else if (list.isError)
    content = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (accounts.length === 0)
    content = (
      <EmptyState title={t("No quota accounts yet")} icon={<Gauge size={26} />}>
        <span>
          {t(
            "Add your Claude, Codex, Grok or DeepSeek accounts to see what is left and when it resets.",
          )}
        </span>
        <button className="xc-btn primary" onClick={() => setDialog({})}>
          {t("Add account")}
        </button>
      </EmptyState>
    );
  else {
    const tight = accounts
      .map((a) => ({ account: a, view: tightestWindow(a, now) }))
      .filter((x) => x.view)
      .sort((a, b) => a.view!.remaining - b.view!.remaining)[0];
    const next = nextReset(accounts, now);
    content = (
      <>
        <StatStrip label={t("AI quotas")}>
          <StatCard
            label={t("Accounts")}
            value={accounts.length}
            tone={failed > 0 ? "warn" : undefined}
            foot={
              failed > 0
                ? `${failed} ${t("with problems")}`
                : t("All readings are fine")
            }
          />
          <StatCard
            label={t("Least left")}
            value={tight ? `${tight.view!.remaining}%` : "—"}
            tone={tight ? tight.view!.tone : undefined}
            foot={
              tight
                ? `${tight.account.name} · ${tight.view!.window.name}`
                : t("No allowance windows")
            }
          />
          <StatCard
            label={t("Next reset")}
            value={
              next
                ? durationText(next.at.getTime() - now.getTime(), language)
                : "—"
            }
            foot={
              next
                ? `${next.account.name} · ${next.window.name}`
                : t("No reset times known")
            }
          />
        </StatStrip>
        {groupByKind(accounts).map((g) => (
          <section
            key={g.kind}
            className="quota-group"
            aria-label={KIND_NAMES[g.kind]}
          >
            <h2 className="quota-group-title">
              {KIND_NAMES[g.kind]} <small>{g.items.length}</small>
            </h2>
            <div className="quota-grid">
              {g.items.map((a, i) => (
                <AccountCard
                  key={a.id}
                  account={a}
                  now={now}
                  refreshing={refreshingId === a.id || refreshingAll}
                  onRefresh={() => refreshOne(a.id)}
                  onEdit={() => setDialog({ account: a })}
                  onDelete={() => onDelete(a)}
                  onMove={{
                    up: i > 0 ? () => move(a.id, -1) : undefined,
                    down:
                      i < g.items.length - 1 ? () => move(a.id, 1) : undefined,
                  }}
                />
              ))}
            </div>
          </section>
        ))}
      </>
    );
  }

  const live = !list.isError && !list.isPending;
  return (
    <div className="xc-page quota-page">
      <PageHeading
        title={t("AI quotas")}
        subtitle={
          live && accounts.length > 0
            ? `${accounts.length} ${t("accounts")}${failed ? ` · ${failed} ${t("with problems")}` : ""}`
            : undefined
        }
        aside={
          live && (
            <>
              {accounts.length > 0 && (
                <button
                  className="xc-btn small"
                  title={t("Refresh all")}
                  aria-label={t("Refresh all")}
                  disabled={refreshingAll}
                  onClick={refreshAll}
                >
                  <RefreshCw size={14} /> {t("Refresh all")}
                </button>
              )}
              <button
                className="xc-btn small"
                title={t("Notifications")}
                aria-label={t("Notifications")}
                onClick={() => setNotifyOpen(true)}
              >
                <Bell size={14} /> {t("Notifications")}
              </button>
              {addButton}
            </>
          )
        }
      />
      {content}
      <NotifyDialog open={notifyOpen} onClose={() => setNotifyOpen(false)} />
      <AccountDialog
        open={dialog !== "closed"}
        account={dialog === "closed" ? undefined : dialog.account}
        onClose={() => setDialog("closed")}
      />
    </div>
  );
}
