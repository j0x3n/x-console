import { useEffect, useRef, useState } from "react";
import { useMatches } from "react-router";
import { Bell, Command, Menu } from "lucide-react";
import { useNotifications } from "../../api/core";
import { useEventConnection } from "../../api/events";
import { useT } from "../../contexts/LanguageContext";
import NotificationsPopover from "./NotificationsPopover";
import TopbarActions from "../../app/TopbarActions";

interface RouteHandle {
  title?: string;
}

export default function Topbar({
  openMobile,
  openPalette,
}: {
  openMobile: () => void;
  openPalette: () => void;
}) {
  const t = useT();
  const matches = useMatches();
  const title =
    [...matches]
      .reverse()
      .map((m) => (m.handle as RouteHandle | undefined)?.title)
      .find(Boolean) ?? "";
  const connected = useEventConnection((s) => s.connected);
  const notifications = useNotifications();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    document.title = title ? `${t(title)} · X Console` : "X Console";
  }, [title, t]);
  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);
  const unread = notifications.data?.unreadCount ?? 0;
  return (
    <header className="topbar">
      <nav className="breadcrumbs" aria-label="Breadcrumb">
        <button
          className="mobile-menu icon-button"
          onClick={openMobile}
          aria-label={t("Open navigation")}
        >
          <Menu size={18} />
        </button>
        <span aria-current="page">{t(title)}</span>
        <span
          className={`xc-dot ${connected ? "ok" : "warn"}`}
          title={connected ? t("Online") : t("Offline")}
        />
      </nav>
      <div className="header-actions">
        <TopbarActions />
        <div
          className="notifications-wrap"
          ref={ref}
          style={{ position: "relative" }}
        >
          <button
            className="icon-button notification-button"
            aria-label={t("Notifications")}
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
          >
            <Bell size={16} />
            {unread > 0 && <i />}
          </button>
          {open && <NotificationsPopover onClose={() => setOpen(false)} />}
        </div>
        <button
          className="command-button"
          onClick={openPalette}
          aria-label={t("Open command palette")}
        >
          <Command size={13} />
          <span>K</span>
        </button>
      </div>
    </header>
  );
}
