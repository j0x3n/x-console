import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useMatches } from "react-router";
import { Bell, Command, Menu } from "lucide-react";
import { useNotifications } from "../../api/core";
import { useEventConnection } from "../../api/events";
import { useT } from "../../contexts/LanguageContext";
import NotificationsPopover from "./NotificationsPopover";
import TopbarActions from "../../app/TopbarActions";
import { usePageTitle } from "../../stores/page-title";
import { navItems } from "../../app/nav";

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
  // 详情页的具体名称，比如项目名。和模块名一样时不重复显示。
  const pageTitle = usePageTitle((s) => s.title);
  const parents = usePageTitle((s) => s.parents);
  const status = usePageTitle((s) => s.status);
  const statusLabel = usePageTitle((s) => s.statusLabel);
  const detail = pageTitle && pageTitle !== t(title) ? pageTitle : "";
  const connected = useEventConnection((s) => s.connected);
  const location = useLocation();
  // 模块首页：导航里同名的入口。找不到（比如 404 页）就不做成链接。
  const moduleTo = navItems.find((n) => n.label === title)?.path;
  const atModule = !moduleTo || (!detail && location.pathname === moduleTo);
  const notifications = useNotifications();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const name = detail || (title ? t(title) : "");
    document.title = name ? `${name} · X Console` : "X Console";
  }, [title, detail, t]);
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
        {atModule ? (
          <span aria-current="page">{t(title)}</span>
        ) : (
          <Link className="breadcrumb-parent" to={moduleTo!}>
            {t(title)}
          </Link>
        )}
        {detail &&
          parents.map((p) => (
            <span key={p.to} className="breadcrumb-mid">
              <span className="breadcrumb-sep">/</span>
              <Link className="breadcrumb-parent" to={p.to}>
                {p.label}
              </Link>
            </span>
          ))}
        {detail && (
          <>
            <span className="breadcrumb-sep">/</span>
            <span aria-current="page" className="breadcrumb-detail">
              {detail}
            </span>
          </>
        )}
        {status && <span className={`xc-dot ${status}`} title={statusLabel} />}
        {!connected && (
          <span
            className="xc-dot warn breadcrumb-offline"
            title={t("Live updates disconnected. Reconnecting.")}
          />
        )}
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
