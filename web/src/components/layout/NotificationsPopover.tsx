import { useNavigate } from "react-router";
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
} from "../../api/core";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { Loading } from "../ui/States";

export default function NotificationsPopover({
  onClose,
}: {
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const list = useNotifications();
  const markRead = useMarkNotificationRead();
  const markAll = useMarkAllNotificationsRead();
  return (
    <div
      className="xc-notifications"
      role="dialog"
      aria-label={t("Notifications")}
    >
      <header>
        <span>{t("Notifications")}</span>
        <button className="xc-btn ghost small" onClick={() => markAll.mutate()}>
          {t("Mark all read")}
        </button>
      </header>
      {list.isPending ? (
        <Loading />
      ) : !list.data?.items.length ? (
        <div className="xc-empty">{t("No notifications")}</div>
      ) : (
        <ul>
          {list.data.items.map((n) => (
            <li key={n.id} className={n.readAt ? "" : "unread"}>
              <button
                onClick={() => {
                  if (!n.readAt) markRead.mutate(n.id);
                  if (n.link) {
                    navigate(n.link);
                    onClose();
                  }
                }}
              >
                <div>
                  <strong>{n.title}</strong>
                  {n.body && <small>{n.body}</small>}
                  <div>
                    <small>{relativeTime(n.createdAt, language)}</small>
                  </div>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
