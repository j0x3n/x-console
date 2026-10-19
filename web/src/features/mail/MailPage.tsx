import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  CheckCheck,
  ListChecks,
  Mail,
  MailOpen,
  Paperclip,
  Plus,
  RefreshCw,
  Settings2,
  Star,
  Trash2,
} from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { SearchBox, Segmented } from "../../components/ui/Toolbar";
import { confirmAction } from "../../components/ui/ConfirmDialog";
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
  useBatchMail,
  useMailAccounts,
  useMailMessages,
  useMarkAllMailRead,
  useSyncMailAccount,
  type MailAccount,
  type MailBatchAction,
  type MailBatchResult,
  type MailFilter,
  type MailSummary,
} from "./api";
import AccountDialog from "./components/AccountDialog";
import MessageView from "./components/MessageView";
import MailRowMenu from "./components/MailRowMenu";
import { mailTime, senderName } from "./logic";
import {
  CATEGORY_FILTERS,
  CATEGORY_LABELS,
  categoryFromParam,
  triageLabel,
} from "./triage";
import { useKeepScroll } from "../../hooks/useKeepScroll";
import { useDraft } from "../../hooks/useDraft";

/*
 * 邮件（B53）：左边收件箱，右边阅读，和笔记一样铺满右侧（B218）。窄屏时一次只显示一栏。
 * 邮箱和未读切换在左栏二级菜单里（2026-10-05），页面里不再放。
 * B219：列表可以多选，批量标已读、未读或删除；顶栏有“全部已读”。
 * XC-66：每封信右键可以单独标已读或未读、归档、删除。
 * 地址参数：?a=账号 id，?m=邮件 id，?unread=1，?new=1 打开添加邮箱。
 */
export default function MailPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const accounts = useMailAccounts();
  const sync = useSyncMailAccount();
  const batch = useBatchMail();
  const readAll = useMarkAllMailRead();
  // B249：搜索框里的字切走再回来还在
  const [q, setQ] = useDraft("mail.search");
  const [query, setQuery] = useState(() => q.trim());
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
  const category = categoryFromParam(params.get("c"));
  const filter: MailFilter = { accountId, unread, q: query, category };
  const list = useMailMessages(filter, accounts.isSuccess);
  const total = (accounts.data ?? []).reduce((n, a) => n + a.unread, 0);

  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [menu, setMenu] = useState<{
    m: MailSummary;
    at: { x: number; y: number };
  } | null>(null);
  const stopSelecting = () => {
    setSelecting(false);
    setSelected(new Set());
  };
  // 换了邮箱、未读或搜索词，之前选的就不算了
  const scopeKey = `${accountId}|${unread}|${query}|${category}`;
  const [lastScope, setLastScope] = useState(scopeKey);
  if (scopeKey !== lastScope) {
    setLastScope(scopeKey);
    setSelected(new Set());
  }

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
  // 出错的邮箱写在列表上方，原来写在页面里的邮箱列
  const broken = accounts.data.filter(
    (a) => a.status === "error" && (!accountId || a.id === accountId),
  );
  const scopeUnread = accountId
    ? (accounts.data.find((a) => a.id === accountId)?.unread ?? 0)
    : total;
  const report = (res: MailBatchResult, done: string) =>
    toast(
      res.errors.length
        ? { message: res.errors.join("\n"), tone: "error" }
        : `${res.done} ${t(done)}`,
    );
  const runBatch = async (
    action: MailBatchAction,
    ids: number[] = [...selected],
  ) => {
    if (!ids.length) return;
    if (
      action === "delete" &&
      !(await confirmAction({
        title: `${t("Delete")} ${ids.length} ${t("mails")}？`,
        description: t("Mail goes to the Trash folder of your mailbox."),
      }))
    )
      return;
    batch.mutate(
      { ids, action },
      {
        onSuccess: (res) => {
          report(
            res,
            action === "delete"
              ? "mails deleted"
              : action === "archive"
                ? "mails archived"
                : action === "read"
                  ? "mails marked as read"
                  : "mails marked as unread",
          );
          if (
            (action === "delete" || action === "archive") &&
            openId &&
            ids.includes(openId)
          )
            set("m", null);
          stopSelecting();
        },
        onError: (e) => toast({ message: e.message, tone: "error" }),
      },
    );
  };
  return (
    <div className={`mail-page${openId ? " is-reading" : ""}`}>
      <PageHeading
        title={
          accounts.data.find((a) => a.id === accountId)?.name ??
          (unread ? t("Unread") : t("All inboxes"))
        }
        subtitle={total ? `${total} ${t("unread")}` : undefined}
        aside={
          <>
            {scopeUnread > 0 && (
              <button
                className="xc-btn small"
                title={t("Mark all read")}
                aria-label={t("Mark all read")}
                disabled={readAll.isPending}
                onClick={() =>
                  readAll.mutate(accountId, {
                    onSuccess: (res) => report(res, "mails marked as read"),
                    onError: (e) =>
                      toast({ message: e.message, tone: "error" }),
                  })
                }
              >
                <CheckCheck size={14} />
              </button>
            )}
            <Link
              className="xc-btn small"
              to="/settings/mail?section=mail-sorting"
              title={t("Mail sorting settings")}
              aria-label={t("Mail sorting settings")}
            >
              <Settings2 size={14} />
            </Link>
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
          </>
        }
      />
      <div className="mail-layout">
        <section className="mail-list" aria-label={t("Inbox")} ref={listRef}>
          {selecting ? (
            <div className="mail-list-tools mail-select-bar">
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={items.length > 0 && selected.size === items.length}
                  onChange={(e) =>
                    setSelected(
                      e.target.checked
                        ? new Set(items.map((m) => m.id))
                        : new Set(),
                    )
                  }
                />
                <span>
                  {selected.size
                    ? `${t("Selected")} ${selected.size}`
                    : t("Select all")}
                </span>
              </label>
              <span className="xc-spacer" />
              <button
                type="button"
                className="xc-btn ghost small"
                title={t("Mark as read")}
                aria-label={t("Mark as read")}
                disabled={!selected.size || batch.isPending}
                onClick={() => runBatch("read")}
              >
                <MailOpen size={14} />
              </button>
              <button
                type="button"
                className="xc-btn ghost small"
                title={t("Mark as unread")}
                aria-label={t("Mark as unread")}
                disabled={!selected.size || batch.isPending}
                onClick={() => runBatch("unread")}
              >
                <Mail size={14} />
              </button>
              <button
                type="button"
                className="xc-btn ghost small mail-danger"
                title={t("Delete")}
                aria-label={t("Delete")}
                disabled={!selected.size || batch.isPending}
                onClick={() => runBatch("delete")}
              >
                <Trash2 size={14} />
              </button>
              <button
                type="button"
                className="xc-btn ghost small"
                onClick={stopSelecting}
              >
                {t("Cancel")}
              </button>
            </div>
          ) : (
            <div className="mail-list-tools">
              <SearchBox
                value={q}
                onChange={setQ}
                placeholder={t("Search sender or subject")}
              />
              {(category || accounts.data.some((a) => a.triage)) && (
                <Segmented<string>
                  label={t("AI sorting")}
                  value={category ?? "all"}
                  onChange={(v) => set("c", v === "all" ? null : v)}
                  options={CATEGORY_FILTERS.map((o) => ({
                    value: o.value,
                    label: t(o.label),
                  }))}
                />
              )}
              {items.length > 0 && (
                <button
                  type="button"
                  className="xc-btn ghost small"
                  title={t("Select mail")}
                  aria-label={t("Select mail")}
                  onClick={() => setSelecting(true)}
                >
                  <ListChecks size={14} />
                </button>
              )}
            </div>
          )}
          {broken.map((a) => (
            <p key={a.id} className="mail-account-error" title={a.lastError}>
              {a.name}：{t("Connection error")}
              {a.lastError ? ` · ${a.lastError}` : ""}
            </p>
          ))}
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
                  selecting={selecting}
                  checked={selected.has(m.id)}
                  onToggle={() =>
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(m.id)) next.delete(m.id);
                      else next.add(m.id);
                      return next;
                    })
                  }
                  onOpen={() => set("m", String(m.id))}
                  onMenu={(at) => setMenu({ m, at })}
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
      {menu && (
        <MailRowMenu
          m={menu.m}
          at={menu.at}
          onClose={() => setMenu(null)}
          onAction={(action) => void runBatch(action, [menu.m.id])}
        />
      )}
    </div>
  );
}

function MessageRow({
  m,
  active,
  account,
  selecting,
  checked,
  onToggle,
  onOpen,
  onMenu,
}: {
  m: MailSummary;
  active: boolean;
  account?: MailAccount;
  selecting: boolean;
  checked: boolean;
  onToggle: () => void;
  onOpen: () => void;
  onMenu: (at: { x: number; y: number }) => void;
}) {
  const t = useT();
  const now = useNow(60_000);
  return (
    <li
      className={`mail-item${selecting ? " selecting" : ""}${checked ? " checked" : ""}`}
    >
      {selecting && (
        <input
          type="checkbox"
          className="mail-row-check"
          checked={checked}
          aria-label={`${t("Select")} ${m.subject || t("(No subject)")}`}
          onChange={onToggle}
        />
      )}
      <button
        type="button"
        className={`mail-row${m.unread ? " unread" : ""}${active ? " active" : ""}`}
        onClick={selecting ? onToggle : onOpen}
        onContextMenu={(e) => {
          if (selecting) return;
          e.preventDefault();
          onMenu({ x: e.clientX, y: e.clientY });
        }}
      >
        <span className="mail-row-top">
          <strong>{senderName(m.from)}</strong>
          {m.flagged && <Star size={12} className="mail-starred" />}
          {m.hasAttachments && <Paperclip size={12} />}
          <small>{mailTime(m.date, now)}</small>
        </span>
        <span className="mail-row-subject">
          {m.triage && (m.triage.category === "action" || m.triage.code) && (
            <span
              className={`xc-badge ${triageLabel(m.triage).tone} mail-row-tag`}
            >
              {t(triageLabel(m.triage).label)}
            </span>
          )}
          {m.triage && m.triage.category !== "action" && !m.triage.code && (
            <small className="mail-row-kind">
              {t(CATEGORY_LABELS[m.triage.category].label)}
            </small>
          )}
          {m.subject || t("(No subject)")}
        </span>
        <span className="mail-row-snippet">
          {account && <i>{account.name}</i>}
          {m.triage?.summary || m.snippet}
        </span>
      </button>
    </li>
  );
}
