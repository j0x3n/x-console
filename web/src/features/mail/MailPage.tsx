import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { Inbox, Mail, Paperclip, Plus, RefreshCw, Star } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { SearchBox } from "../../components/ui/Toolbar";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useNow } from "../calendar/hooks";
import {
  useMailAccounts,
  useMailMessages,
  useSyncMailAccount,
  type MailAccount,
  type MailFilter,
  type MailSummary,
} from "./api";
import AccountDialog from "./components/AccountDialog";
import MessageView from "./components/MessageView";
import { mailTime, senderName } from "./logic";
import { useKeepScroll } from "../../hooks/useKeepScroll";

/*
 * 邮件（B53）：左边账号，中间收件箱，右边阅读。窄屏时一次只显示一栏。
 * 地址参数：?a=账号 id，?m=邮件 id，?unread=1，?new=1 打开添加邮箱。
 */
export default function MailPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const accounts = useMailAccounts();
  const sync = useSyncMailAccount();
  const [q, setQ] = useState("");
  const [query, setQuery] = useState("");
  useEffect(() => {
    const id = setTimeout(() => setQuery(q.trim()), 300);
    return () => clearTimeout(id);
  }, [q]);
  const set = (key: string, value: string | null) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value === null) next.delete(key);
        else next.set(key, value);
        return next;
      },
      { replace: key !== "m" },
    );
  const accountId = Number(params.get("a")) || null;
  const openId = Number(params.get("m")) || null;
  // B80：列表和正文的滚动位置
  const listRef = useRef<HTMLElement>(null);
  const readRef = useRef<HTMLElement>(null);
  const listKey = new URLSearchParams(params);
  listKey.delete("m");
  useKeepScroll(listRef, `mail.list:${listKey.toString()}`);
  useKeepScroll(readRef, `mail.read:${openId ?? ""}`);
  const unread = params.get("unread") === "1";
  const filter: MailFilter = { accountId, unread, q: query };
  const list = useMailMessages(filter, accounts.isSuccess);
  const total = (accounts.data ?? []).reduce((n, a) => n + a.unread, 0);

  if (accounts.isPending) return <Loading />;
  if (accounts.isError)
    return isNotLive(accounts.error) ? (
      <div className="xc-page">
        <PageHeading title={t("Mail")} />
        <NotLive name={t("Mail")} icon={<Mail size={28} />} />
      </div>
    ) : (
      <ErrorState error={accounts.error} onRetry={() => accounts.refetch()} />
    );

  const adding = params.get("new") === "1";
  const dialog = (
    <AccountDialog open={adding} onClose={() => set("new", null)} />
  );
  if (accounts.data.length === 0)
    return (
      <div className="xc-page">
        <PageHeading title={t("Mail")} />
        <EmptyState title={t("No mailboxes yet")} icon={<Mail size={28} />}>
          <span>
            {t(
              "Add Gmail or an enterprise mailbox. New mail shows up here within seconds.",
            )}
          </span>
          <button
            className="xc-btn primary small"
            onClick={() => set("new", "1")}
          >
            <Plus size={14} /> {t("Add mailbox")}
          </button>
        </EmptyState>
        {dialog}
      </div>
    );

  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <div className={`xc-page wide mail-page${openId ? " is-reading" : ""}`}>
      <PageHeading
        title={t("Mail")}
        subtitle={total ? `${total} ${t("unread")}` : undefined}
        aside={
          <>
            <button
              className="xc-btn small"
              title={t("Sync now")}
              aria-label={t("Sync now")}
              onClick={() => {
                for (const a of accounts.data)
                  if (!accountId || a.id === accountId) sync.mutate(a.id);
                toast(t("Syncing mail"));
              }}
            >
              <RefreshCw size={14} />
            </button>
            <button
              className="xc-btn small"
              title={t("Add mailbox")}
              onClick={() => set("new", "1")}
            >
              <Plus size={14} /> {t("Add mailbox")}
            </button>
          </>
        }
      />
      <div className="mail-layout">
        <nav className="mail-accounts" aria-label={t("Mailboxes")}>
          <AccountLink
            active={accountId === null}
            icon={<Inbox size={15} />}
            label={t("All inboxes")}
            unread={total}
            onClick={() => set("a", null)}
          />
          {accounts.data.map((a) => (
            <AccountLink
              key={a.id}
              active={accountId === a.id}
              account={a}
              label={a.name}
              unread={a.unread}
              onClick={() => set("a", String(a.id))}
            />
          ))}
        </nav>
        <section className="mail-list" aria-label={t("Inbox")} ref={listRef}>
          <div className="mail-list-tools">
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search sender or subject")}
            />
            <button
              type="button"
              className={`xc-btn small${unread ? " on" : ""}`}
              aria-pressed={unread}
              onClick={() => set("unread", unread ? null : "1")}
            >
              {t("Unread")}
            </button>
          </div>
          {list.isPending ? (
            <Loading />
          ) : list.isError ? (
            <ErrorState error={list.error} onRetry={() => list.refetch()} />
          ) : items.length === 0 ? (
            <p className="mail-empty">
              {unread || query ? t("Nothing matches") : t("Inbox is empty")}
            </p>
          ) : (
            <ul>
              {items.map((m) => (
                <MessageRow
                  key={m.id}
                  m={m}
                  active={m.id === openId}
                  account={
                    accountId === null && accounts.data.length > 1
                      ? accounts.data.find((a) => a.id === m.accountId)
                      : undefined
                  }
                  onOpen={() => set("m", String(m.id))}
                />
              ))}
              {list.hasNextPage && (
                <li className="mail-more">
                  <button
                    type="button"
                    className="xc-btn small ghost"
                    disabled={list.isFetchingNextPage}
                    onClick={() => list.fetchNextPage()}
                  >
                    {t("Load more")}
                  </button>
                </li>
              )}
            </ul>
          )}
        </section>
        <section className="mail-read" ref={readRef}>
          {openId ? (
            <MessageView id={openId} onBack={() => set("m", null)} />
          ) : (
            <p className="mail-empty">{t("Pick a mail to read")}</p>
          )}
        </section>
      </div>
      {dialog}
    </div>
  );
}

function AccountLink({
  active,
  account,
  icon,
  label,
  unread,
  onClick,
}: {
  active: boolean;
  account?: MailAccount;
  icon?: React.ReactNode;
  label: string;
  unread: number;
  onClick: () => void;
}) {
  const t = useT();
  const tone = !account
    ? ""
    : account.status === "ok"
      ? "ok"
      : account.status === "error"
        ? "danger"
        : "warn";
  return (
    <button
      type="button"
      className={active ? "active" : ""}
      onClick={onClick}
      title={account?.status === "error" ? account.lastError : account?.email}
    >
      {icon ?? <span className={`xc-dot ${tone}`} />}
      <span>{label}</span>
      {account?.status === "error" && (
        <small className="mail-account-error">{t("Connection error")}</small>
      )}
      {unread > 0 && <em>{unread}</em>}
    </button>
  );
}

function MessageRow({
  m,
  active,
  account,
  onOpen,
}: {
  m: MailSummary;
  active: boolean;
  account?: MailAccount;
  onOpen: () => void;
}) {
  const t = useT();
  const now = useNow(60_000);
  return (
    <li>
      <button
        type="button"
        className={`mail-row${m.unread ? " unread" : ""}${active ? " active" : ""}`}
        onClick={onOpen}
      >
        <span className="mail-row-top">
          <strong>{senderName(m.from)}</strong>
          {m.flagged && <Star size={12} className="mail-starred" />}
          {m.hasAttachments && <Paperclip size={12} />}
          <small>{mailTime(m.date, now)}</small>
        </span>
        <span className="mail-row-subject">
          {m.subject || t("(No subject)")}
        </span>
        <span className="mail-row-snippet">
          {account && <i>{account.name}</i>}
          {m.snippet}
        </span>
      </button>
    </li>
  );
}
