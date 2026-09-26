import type * as Model from "../../types/domain";
import React from "react";
import AgentBadge from "../ui/AgentBadge";

interface NotificationsPopoverProps {
  notificationsClosing: boolean;
  t: Model.Translate;
  language: Model.Language;
  decisions: Model.Decision[];
  navigate: Model.Navigate;
  setNotificationsOpen: Model.Setter<boolean>;
}

export default function NotificationsPopover({
  notificationsClosing,
  t,
  language,
  decisions,
  navigate,
  setNotificationsOpen,
}: NotificationsPopoverProps) {
  return (
    <div
      className={
        "notifications-popover" + (notificationsClosing ? " is-closing" : "")
      }
      role="dialog"
      aria-label={t("Notifications")}
    >
      <div className="popover-head">
        <b>{t("Notifications")}</b>
        <span>{language === "zh" ? "最近 24 小时" : "Last 24 hours"}</span>
      </div>
      <div className="notification-list">
        {[
          {
            agent: "Pilot",
            en: "Pilot wants to move Brightwell Labs to Negotiation",
            zh: "Pilot 建议将 Brightwell Labs 移至谈判阶段",
            time: "1h",
            timeZh: "1 小时前",
            unread: decisions.some((item) => item.id === 3),
            view: "Today",
          },
          {
            agent: "Scribe",
            en: "Scribe drafted a follow-up to Priya Raman",
            zh: "Scribe 起草了给 Priya Raman 的跟进邮件",
            time: "2h",
            timeZh: "2 小时前",
            unread: decisions.some((item) => item.id === 1),
            view: "Today",
          },
          {
            agent: "Ledger",
            en: "Ledger flagged renewal risk at Oakline Freight",
            zh: "Ledger 发现 Oakline Freight 续约风险",
            time: "2h",
            timeZh: "2 小时前",
            unread: decisions.some((item) => item.id === 2),
            view: "Today",
          },
          {
            initials: "TP",
            en: "Theo Park mentioned you on Halcyon Robotics",
            zh: "Theo Park 在 Halcyon Robotics 中提到了你",
            time: "yesterday",
            timeZh: "昨天",
            view: "Halcyon Robotics",
          },
          {
            initials: "KM",
            en: "Kofi Mensah qualified Fernbrook and booked a demo",
            zh: "Kofi Mensah 完成 Fernbrook 初步评估并预约演示",
            time: "yesterday",
            timeZh: "昨天",
            view: "Fernbrook",
          },
        ].map((notice) => (
          <button
            className="notification-entry"
            key={notice.en}
            onClick={() => {
              navigate(notice.view);
              setNotificationsOpen(false);
            }}
          >
            {notice.agent ? (
              <AgentBadge name={notice.agent} size="small" />
            ) : (
              <span className="teammate-avatar">{notice.initials}</span>
            )}
            <span className="notification-title">
              {language === "zh" ? notice.zh : notice.en}
            </span>
            <time>{language === "zh" ? notice.timeZh : notice.time}</time>
            {notice.unread && (
              <i aria-label={language === "zh" ? "未读" : "Unread"} />
            )}
          </button>
        ))}
      </div>
    </div>
  );
}
