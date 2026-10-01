import { Link } from "react-router";
import { CalendarClock, Globe, Mail, Receipt, ShieldCheck } from "lucide-react";
import { ApiError } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { useNow } from "../../calendar/hooks";
import { useMailSummary } from "../../mail/api";
import { mailTime, senderName } from "../../mail/logic";
import { useMonitors, useSubscriptions } from "../../monitoring/api";
import { attentionItems, type AttentionItem } from "../attention";
import { Empty, Pending, QueryState } from "./shared";

const LIMIT = 4;

const notLive = (e: unknown) =>
  e instanceof ApiError && (e.status === 404 || e.status === 501);

/** 今日页“邮件”卡片（B59）：最近几封未读。 */
export function MailCard() {
  const t = useT();
  const now = useNow(60_000);
  const summary = useMailSummary();
  if (summary.isPending) return <Pending />;
  if (summary.isError)
    return notLive(summary.error) ? (
      <Empty>{t("Mail is not live yet")}</Empty>
    ) : (
      <QueryState query={summary} />
    );
  const s = summary.data;
  if (s.accounts.length === 0)
    return (
      <Empty
        action={
          <Link className="xc-btn small" to="/mail?new=1">
            {t("Add mailbox")}
          </Link>
        }
      >
        {t("No mailboxes yet")}
      </Empty>
    );
  const broken = s.accounts.filter((a) => a.status === "error");
  return (
    <>
      {broken.length > 0 && (
        <div className="today-note is-error">
          <span>
            {broken.map((a) => a.name).join("、")} {t("cannot connect")}
          </span>
          <Link className="xc-btn small ghost" to="/settings/mail">
            {t("Go to settings")}
          </Link>
        </div>
      )}
      {s.latest.length === 0 ? (
        <Empty>{t("No unread mail")}</Empty>
      ) : (
        <div className="xc-list">
          {s.latest.slice(0, LIMIT).map((m) => (
            <Link className="today-row" key={m.id} to={`/mail?m=${m.id}`}>
              <span className="today-row-icon accent">
                <Mail size={15} />
              </span>
              <span className="today-row-main">
                <strong>{m.subject || t("(No subject)")}</strong>
                <small>
                  {senderName(m.from)} · {mailTime(m.date, now)}
                </small>
              </span>
            </Link>
          ))}
          {s.unread > LIMIT && (
            <Link className="today-row today-row-more" to="/mail?unread=1">
              <span className="today-row-main">
                <small>
                  {t("All unread")} {s.unread}
                </small>
              </span>
            </Link>
          )}
        </div>
      )}
    </>
  );
}

const kindIcons = {
  site: Globe,
  tls: ShieldCheck,
  domain: CalendarClock,
  subscription: Receipt,
};

/** 今日页“监控”卡片（B59）：挂掉的网站、快到期的证书域名、快续费的订阅。 */
export function MonitoringCard() {
  const t = useT();
  const monitors = useMonitors();
  const subs = useSubscriptions(false);
  if (monitors.isPending || subs.isPending) return <Pending />;
  if (monitors.isError) return <QueryState query={monitors} />;
  const items = attentionItems(monitors.data, subs.data ?? []);
  if (items.length === 0)
    return (
      <Empty>{t("Websites, certificates and renewals are all fine")}</Empty>
    );
  return (
    <div className="xc-list">
      {items.slice(0, LIMIT + 1).map((x) => (
        <AttentionRow key={x.key} x={x} />
      ))}
    </div>
  );
}

function AttentionRow({ x }: { x: AttentionItem }) {
  const t = useT();
  const Icon = kindIcons[x.kind];
  const what =
    x.kind === "site"
      ? t("Website is down")
      : x.kind === "subscription"
        ? x.days! < 0
          ? `${t("Renewal overdue")} ${-x.days!} ${t("days")}`
          : x.days === 0
            ? t("Renews today")
            : `${x.days} ${t("days to renewal")}`
        : x.days! <= 0
          ? t("Expired")
          : `${x.kind === "tls" ? t("Certificate") : t("Domain")} ${x.days} ${t("days left")}`;
  return (
    <Link className="today-row" to={x.link}>
      <span className={`today-row-icon today-tone-${x.tone}`}>
        <Icon size={15} />
      </span>
      <span className="today-row-main">
        <strong>{x.name}</strong>
        <small className={`today-tone-text-${x.tone}`}>{what}</small>
      </span>
    </Link>
  );
}
