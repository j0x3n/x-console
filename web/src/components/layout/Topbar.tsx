import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useMatches } from "react-router";
import { Bell, PanelLeftOpen, Settings2 } from "lucide-react";
import { useNotifications } from "../../api/core";
import { useEventConnection } from "../../api/events";
import { useT } from "../../contexts/LanguageContext";
import NotificationsPopover from "./NotificationsPopover";
import TopbarActions from "../../app/TopbarActions";
import { usePageTitle } from "../../stores/page-title";
import { moduleOfPath, useModules } from "../../app/modules";
import { useSidebar } from "../../stores/sidebar";
import { PageActionsSlot } from "./PageActions";
import { navItems } from "../../app/nav";
import { useCurrentNavPanel } from "./Sidebar";

interface RouteHandle {
  title?: string;
}

export default function Topbar({
  openMobile,
}: {
  openMobile: () => void;
  /** B100 起顶栏不再放命令面板按钮（图标栏里有搜索），保留参数兼容 */
  openPalette?: () => void;
}) {
  const t = useT();
  const matches = useMatches();
  const location = useLocation();
  // B57：被隐藏的模块按不存在的页面显示，标题也不能露出模块名
  const modules = useModules();
  const hiddenHere = !modules.has(moduleOfPath(location.pathname));
  const title = hiddenHere
    ? modules.pending
      ? ""
      : "Not found"
    : ([...matches]
        .reverse()
        .map((m) => (m.handle as RouteHandle | undefined)?.title)
        .find(Boolean) ?? "");
  // 详情页的具体名称，比如项目名。和模块名一样时不重复显示。
  const pageTitle = usePageTitle((s) => s.title);
  const parents = usePageTitle((s) => s.parents);
  const status = usePageTitle((s) => s.status);
  const statusLabel = usePageTitle((s) => s.statusLabel);
  const subtitle = usePageTitle((s) => s.subtitle);
  const mark = usePageTitle((s) => s.mark);
  const collapsed = useSidebar((s) => s.collapsed);
  const toggleSidebar = useSidebar((s) => s.toggle);
  // B103：收起按钮在二级菜单里。桌面上只有收起了二级菜单才在这里放展开按钮；
  // 二级菜单显示时，模块名已经在它的标题栏里，这里不再重复（见 ui.css 的 .panel-open）。
  const { Panel } = useCurrentNavPanel();
  const hasPanel = !!Panel && !hiddenHere;
  const panelOpen = hasPanel && !collapsed;
  const detail = pageTitle && pageTitle !== t(title) ? pageTitle : "";
  const connected = useEventConnection((s) => s.connected);
  // 模块首页：导航里同名的入口。找不到（比如 404 页）就不做成链接。
  const moduleItem = navItems.find((n) => n.label === title);
  const moduleTo = moduleItem?.path;
  // 设置不在左栏的模块列表里（入口在图标栏底部），单独给图标
  const ModuleIcon =
    moduleItem?.icon ?? (title === "Settings" ? Settings2 : undefined);
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
    <header className={`topbar${panelOpen ? " panel-open" : ""}`}>
      <nav className="breadcrumbs" aria-label="Breadcrumb">
        {/* 桌面上展开收起的二级菜单，手机上打开抽屉（B20、B103） */}
        <button
          className={`sidebar-toggle icon-button${hasPanel && collapsed ? "" : " mobile-only"}`}
          onClick={() =>
            window.matchMedia("(max-width: 720px)").matches
              ? openMobile()
              : toggleSidebar()
          }
          aria-label={t("Expand sidebar")}
          title={`${t("Expand sidebar")} (⌘B)`}
        >
          <span className="sidebar-toggle-desktop">
            <PanelLeftOpen size={16} />
          </span>
          <span className="sidebar-toggle-mobile">
            <PanelLeftOpen size={17} />
          </span>
        </button>
        {/* B100：页面名前面的模块图标 */}
        {ModuleIcon && (
          <span className="topbar-icon topbar-module" aria-hidden>
            <ModuleIcon size={15} strokeWidth={1.8} />
          </span>
        )}
        {atModule ? (
          <span className="topbar-module" aria-current="page">
            {t(title)}
          </span>
        ) : (
          <Link className="breadcrumb-parent topbar-module" to={moduleTo!}>
            {t(title)}
          </Link>
        )}
        {detail &&
          parents.map((p, i) => (
            <span key={p.to} className="breadcrumb-mid">
              <span
                className={`breadcrumb-sep${i === 0 ? " topbar-module" : ""}`}
              >
                /
              </span>
              <Link className="breadcrumb-parent" to={p.to}>
                {p.label}
              </Link>
            </span>
          ))}
        {detail && (
          <>
            <span
              className={`breadcrumb-sep${parents.length ? "" : " topbar-module"}`}
            >
              /
            </span>
            {mark}
            <span aria-current="page" className="breadcrumb-detail">
              {detail}
            </span>
          </>
        )}
        {status && (
          // 离线、异常时把状态直接写在小点后面，不用悬停才看得到
          <span className={`breadcrumb-status ${status}`} title={statusLabel}>
            <span className={`xc-dot ${status}`} />
            {status !== "ok" && statusLabel && <small>{statusLabel}</small>}
          </span>
        )}
        {!connected && (
          <span
            className="xc-dot warn breadcrumb-offline"
            title={t("Live updates disconnected. Reconnecting.")}
          />
        )}
        {subtitle && (
          <span className={`topbar-subtitle${detail ? "" : " after-module"}`}>
            {subtitle}
          </span>
        )}
      </nav>
      <div className="header-actions">
        <PageActionsSlot />
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
      </div>
    </header>
  );
}
