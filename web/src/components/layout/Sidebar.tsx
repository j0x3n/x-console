import { Fragment, useEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router";
import {
  LogOut,
  Moon,
  PanelLeftClose,
  Palette,
  Search,
  Settings2,
  Languages,
  type LucideIcon,
} from "lucide-react";
import { navGroupLabels, navItems, type NavGroup } from "../../app/nav";
import { moduleOfPath, useModules } from "../../app/modules";
import { useAuthStatus, useLogout } from "../../api/core";
import { useT } from "../../contexts/LanguageContext";
import { accents, usePreferencesStore } from "../../stores/preferences-store";
import type { Accent } from "../../types/domain";
import {
  InstallHelpDialog,
  InstallMenuItem,
} from "../../features/pwa/InstallMenu";
import { useNavChildren } from "../../lib/navChildren";
import {
  badgeLabel,
  useNavExtras,
  type NavAction,
  type NavBadgeHook,
  type NavIconHook,
  type NavStatusHook,
} from "../../lib/navBadges";
import { lastPathFor } from "../../hooks/useKeepScroll";
import { useSidebar } from "../../stores/sidebar";

interface SidebarProps {
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  openPalette: () => void;
}

/*
 * 左栏（B99）：最左边一条图标栏放全部模块，右边是当前模块的二级菜单。
 * 二级菜单只有登记过 registerNavChildren 的模块才有，⌘B 收起或展开。
 * 手机上整个左栏是一个抽屉，图标栏和二级菜单一起出来。
 */
export default function Sidebar({
  mobileOpen,
  setMobileOpen,
  openPalette,
}: SidebarProps) {
  const t = useT();
  const extras = useNavExtras();
  const navigate = useNavigate();
  const panelHidden = useSidebar((s) => s.collapsed);
  const togglePanel = useSidebar((s) => s.toggle);
  const location = useLocation();
  // B57：锁定时被隐藏的模块不出现，分组空了整组不显示
  const modules = useModules();
  const { current, currentPath, Panel } = useCurrentNavPanel();
  const groups = (Object.keys(navGroupLabels) as NavGroup[])
    .map((group) => ({
      group,
      items: navItems.filter(
        (item) => item.group === group && modules.has(moduleOfPath(item.path)),
      ),
    }))
    .filter((g) => g.items.length > 0);
  const action = current ? extras.actions[current.path] : undefined;
  const showPanel = !!current && !!Panel && (mobileOpen || !panelHidden);
  const close = () => setMobileOpen(false);
  return (
    <>
      {mobileOpen && (
        <button
          className="mobile-scrim"
          aria-label={t("Close navigation")}
          onClick={close}
        />
      )}
      <div className={`sidebar${mobileOpen ? " open" : ""}`}>
        <nav className="nav-rail" aria-label={t("Main")}>
          {/* 点 Logo 回今日，同时发出事件。隐藏内容模块（B13）数连续点击次数。 */}
          <button
            type="button"
            className="nav-rail-brand"
            aria-label="X Console"
            onClick={() => {
              window.dispatchEvent(new Event("xc:brand-tap"));
              navigate("/");
              close();
            }}
          >
            X
          </button>
          {/* 隐藏内容解锁时，VaultPanel 把锁定按钮放到这里 */}
          <span id="brand-slot" className="brand-slot" />
          <button
            type="button"
            className="nav-rail-item"
            aria-label={t("Search or run a command...")}
            onClick={openPalette}
          >
            <Search size={18} strokeWidth={1.6} />
            <span className="nav-rail-tip">
              {t("Search")} <kbd>⌘K</kbd>
            </span>
          </button>
          {groups.map(({ group, items }, index) => (
            <Fragment key={group}>
              <span
                className={`nav-rail-sep${index === 0 ? " first" : ""}`}
                aria-hidden
              />
              {items.map((item) => (
                <NavLink
                  key={item.path}
                  to={
                    item.path === "/"
                      ? "/"
                      : lastPathFor(item.path, location.pathname)
                  }
                  aria-label={t(item.label)}
                  onClick={close}
                  className={`nav-rail-item${item.path === currentPath ? " selected" : ""}`}
                >
                  {extras.icons[item.path] ? (
                    <NavIconMark
                      hook={extras.icons[item.path]}
                      fallback={item.icon}
                    />
                  ) : (
                    <item.icon size={18} strokeWidth={1.6} />
                  )}
                  {extras.badges[item.path] ? (
                    <NavBadgeMark hook={extras.badges[item.path]} />
                  ) : (
                    extras.statuses[item.path] && (
                      <NavStatusDot hook={extras.statuses[item.path]} />
                    )
                  )}
                  <span className="nav-rail-tip">
                    {t(item.label)}
                    {extras.statuses[item.path] && (
                      <NavStatusText hook={extras.statuses[item.path]} />
                    )}
                  </span>
                </NavLink>
              ))}
            </Fragment>
          ))}
          <span className="nav-rail-spacer" />
          <NavLink
            to="/settings"
            aria-label={t("Settings")}
            onClick={close}
            className={({ isActive }) =>
              `nav-rail-item${isActive ? " selected" : ""}`
            }
          >
            <Settings2 size={18} strokeWidth={1.6} />
            <span className="nav-rail-tip">{t("Settings")}</span>
          </NavLink>
          <ProfileMenu />
        </nav>
        {showPanel && (
          <aside className="nav-panel" aria-label={t(current.label)}>
            <div className="nav-panel-head">
              <span>{t(current.label)}</span>
              {action && (
                <NavActionButton
                  action={action}
                  onRun={() => {
                    close();
                    action.run(navigate);
                  }}
                />
              )}
              {/* B103：收起按钮放在二级菜单里，收起后顶栏左边出现展开按钮。手机上不显示 */}
              <button
                type="button"
                className="nav-panel-action nav-panel-collapse"
                aria-label={t("Collapse sidebar")}
                title={`${t("Collapse sidebar")} (⌘B)`}
                onClick={togglePanel}
              >
                <PanelLeftClose size={16} />
              </button>
            </div>
            <div className="nav-panel-body nav-children">
              <Panel onNavigate={close} />
            </div>
          </aside>
        )}
      </div>
    </>
  );
}

/**
 * 当前模块和它登记的二级菜单。顶栏也要用：有二级菜单时才显示展开按钮，
 * 二级菜单显示时顶栏不再重复模块名（B103）。
 */
export function useCurrentNavPanel() {
  const children = useNavChildren();
  const location = useLocation();
  const modules = useModules();
  const currentPath = moduleForPath(location.pathname);
  const current = navItems.find(
    (n) => n.path === currentPath && modules.has(moduleOfPath(n.path)),
  );
  const Panel = current ? children[current.path] : undefined;
  return { current, currentPath, Panel };
}

/** 图标右上角的数量（B76）。单独一个组件，登记的 Hook 在这里调用，顺序固定。 */
function NavBadgeMark({ hook }: { hook: NavBadgeHook }) {
  const badge = hook();
  if (!badge || badge.count <= 0) return null;
  const label = badgeLabel(badge.count);
  return (
    <i
      className={`nav-rail-badge ${badge.tone ?? "danger"}${label ? "" : " dot"}`}
      title={badge.title}
      aria-label={badge.title}
    >
      {label}
    </i>
  );
}

/** 图标：模块可以换图标，或者让它跳动表示正在工作（B86、B88）。 */
function NavIconMark({
  hook,
  fallback,
}: {
  hook: NavIconHook;
  fallback: LucideIcon;
}) {
  const state = hook();
  const Icon = state?.icon ?? fallback;
  return (
    <i
      className={`nav-icon${state?.state === "working" ? " is-working" : ""}`}
      title={state?.title}
      aria-hidden
    >
      <Icon size={18} strokeWidth={1.6} />
    </i>
  );
}

/** 路由器这类状态（B93）：图标右上角一个点，文字放进悬停提示。 */
function NavStatusDot({ hook }: { hook: NavStatusHook }) {
  const status = hook();
  if (!status?.dot) return null;
  return <i className={`nav-rail-status ${status.dot}`} aria-hidden />;
}

function NavStatusText({ hook }: { hook: NavStatusHook }) {
  const status = hook();
  const text = status?.text || status?.title;
  return text ? <small>{text}</small> : null;
}

/** 二级菜单标题栏右边的按钮，比如笔记的“+”（B72）。 */
function NavActionButton({
  action,
  onRun,
}: {
  action: NavAction;
  onRun: () => void;
}) {
  const t = useT();
  const Icon = action.icon;
  return (
    <button
      type="button"
      className="nav-panel-action"
      aria-label={t(action.label)}
      title={t(action.label)}
      onClick={onRun}
    >
      <Icon size={16} />
    </button>
  );
}

const accentLabels: Record<Accent, string> = {
  indigo: "Indigo",
  ocean: "Ocean",
  teal: "Teal",
  violet: "Violet",
  rose: "Rose",
  graphite: "Graphite",
};

function ProfileMenu() {
  const t = useT();
  const navigate = useNavigate();
  const auth = useAuthStatus();
  const logout = useLogout();
  const { language, setLanguage, themeMode, setThemeMode, accent, setAccent } =
    usePreferencesStore();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);
  const username = auth.data?.username ?? "";
  return (
    <div className="profile-menu-wrap" ref={ref}>
      <InstallHelpDialog />
      {open && (
        <div className="profile-popover" role="menu">
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => {
                setOpen(false);
                navigate("/settings");
              }}
            >
              <Settings2 size={15} />
              <span>{t("Settings")}</span>
            </button>
          </div>
          <div className="profile-menu-section">
            {/* 和语言一样：点一次换一个，当前值写在右边 */}
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => setThemeMode(nextThemeMode[themeMode])}
            >
              <Moon size={15} />
              <span>{t("Night mode")}</span>
              <small>{t(themeModeLabels[themeMode])}</small>
            </button>
            <div className="profile-menu-row">
              <Palette size={15} />
              <span>{t("Theme color")}</span>
            </div>
            <div
              className="profile-accents"
              role="radiogroup"
              aria-label={t("Theme color")}
            >
              {accents.map((a) => (
                <button
                  key={a}
                  role="radio"
                  aria-checked={accent === a}
                  aria-label={t(accentLabels[a])}
                  title={t(accentLabels[a])}
                  className={`profile-accent accent-${a}${accent === a ? " active" : ""}`}
                  onClick={() => setAccent(a)}
                />
              ))}
            </div>
          </div>
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => setLanguage(language === "zh" ? "en" : "zh")}
            >
              <Languages size={15} />
              <span>{t("Language")}</span>
              <small>{language === "zh" ? "中文" : "English"}</small>
            </button>
            <InstallMenuItem onDone={() => setOpen(false)} />
          </div>
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => logout.mutate()}
            >
              <LogOut size={15} />
              <span>{t("Sign out")}</span>
            </button>
          </div>
        </div>
      )}
      <button
        className="nav-rail-avatar"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`${t("Account")} ${username}`}
        title={username}
        onClick={() => setOpen((v) => !v)}
      >
        {username.slice(0, 1).toUpperCase()}
      </button>
    </div>
  );
}

/** 夜间模式点一次换一个：关 → 开 → 自动 → 关。 */
const nextThemeMode = {
  light: "dark",
  dark: "system",
  system: "light",
} as const;

const themeModeLabels = { dark: "On", light: "Off", system: "Auto" } as const;

/**
 * 当前地址属于哪个一级菜单。早报的地址在 /calendar 下面，但属于今日页（B66）。
 * 导出给测试用。
 */
export function moduleForPath(pathname: string): string | undefined {
  if (pathname === "/" || pathname.startsWith("/calendar/briefs")) return "/";
  return navItems
    .filter((n) => n.path !== "/")
    .find((n) => pathname === n.path || pathname.startsWith(`${n.path}/`))
    ?.path;
}
