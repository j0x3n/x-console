import { useCallback, useEffect, useRef, useState } from "react";
import type { SyntheticEvent } from "react";
import { NavLink, useLocation, useNavigate } from "react-router";
import {
  LayoutGrid,
  LogOut,
  Moon,
  PanelLeftClose,
  Palette,
  Search,
  Settings2,
  Languages,
  type LucideIcon,
} from "lucide-react";
import { entryOf, navItems, navMerges } from "../../app/nav";
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
import BrandMark from "../ui/BrandMark";
import Launcher from "./Launcher";
import { NavPanelGroup, NavPanelLink } from "./NavPanel";
import {
  buildEntries,
  isPinned,
  railEntries,
  togglePin,
  type NavEntry,
} from "./navEntries";

/** 每个合并组上次看的是哪个功能，点组的入口时回到那里（B179）。 */
const lastMember = new Map<string, string>();

/** 入口要跳去的地址：组里上次看的功能，再加上那个功能里上次看的页面。 */
function entryTarget(entry: NavEntry, pathname: string): string {
  const remembered = lastMember.get(entry.path);
  const member =
    entry.members.find((m) => m.path === remembered) ?? entry.members[0];
  return lastPathFor(member.path, pathname);
}

/** 组里第一个登记了这种标记（数量、状态、图标）的功能。 */
function extraKey(entry: NavEntry, map: Record<string, unknown>) {
  return entry.members.map((m) => m.path).find((p) => map[p]);
}

/** 图标栏会滚动，提示是 fixed 定位，悬停或聚焦时把图标中线的纵坐标告诉它。 */
function placeTip(event: SyntheticEvent<HTMLElement>) {
  const item = (event.target as HTMLElement).closest<HTMLElement>(
    ".nav-rail-item",
  );
  if (!item) return;
  const rect = item.getBoundingClientRect();
  item.style.setProperty("--tip-y", `${rect.top + rect.height / 2}px`);
}

interface SidebarProps {
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  openPalette: () => void;
}

/*
 * 左栏（B99）：最左边一条图标栏，右边是当前模块的二级菜单。
 * B179：图标栏只放固定的常用入口，其余在“全部”打开的启动台里；几组相近的功能合并成一个入口。
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
  const { currentPath, Panel, panelLabel } = useCurrentNavPanel();
  const visible = navItems.filter((item) =>
    modules.has(moduleOfPath(item.path)),
  );
  const home = visible.find((item) => item.path === "/");
  const entries = buildEntries(visible);
  const pins = usePreferencesStore((s) => s.navPins);
  const setPins = usePreferencesStore((s) => s.setNavPins);
  const rail = railEntries(entries, pins, currentPath);
  const currentEntry = currentPath ? entryOf(currentPath) : undefined;
  const [launcher, setLauncher] = useState(false);
  const closeLauncher = useCallback(() => setLauncher(false), []);
  useEffect(() => {
    if (currentPath) lastMember.set(entryOf(currentPath), currentPath);
  }, [currentPath]);
  const action = currentPath ? extras.actions[currentPath] : undefined;
  const showPanel = !!Panel && (mobileOpen || !panelHidden);
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
        <nav
          className="nav-rail"
          aria-label={t("Main")}
          onPointerOver={placeTip}
          onFocus={placeTip}
        >
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
            <BrandMark size="100%" />
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
          {/* B178：站标、搜索、今日固定在顶部，头像固定在底部，只有中间的模块滚动。窗口矮时回今日不用往回滚 */}
          {home && (
            <RailLink
              to="/"
              label={home.label}
              icon={home.icon}
              selected={currentPath === "/"}
              keys={{
                icon: extras.icons["/"] ? "/" : undefined,
                badge: extras.badges["/"] ? "/" : undefined,
                status: extras.statuses["/"] ? "/" : undefined,
              }}
              extras={extras}
              onClose={close}
            />
          )}
          <span className="nav-rail-sep first" aria-hidden />
          <div className="nav-rail-scroll">
            {rail.map((entry) => (
              <RailLink
                key={entry.path}
                to={entryTarget(entry, location.pathname)}
                label={entry.label}
                icon={entry.icon}
                selected={entry.path === currentEntry}
                keys={{
                  icon: extraKey(entry, extras.icons),
                  badge: extraKey(entry, extras.badges),
                  status: extraKey(entry, extras.statuses),
                }}
                extras={extras}
                onClose={close}
              />
            ))}
          </div>
          {/* B179：全部功能在启动台里，可以固定到左栏 */}
          <button
            type="button"
            className={`nav-rail-item nav-rail-all${launcher ? " selected" : ""}`}
            aria-label={t("All apps")}
            aria-haspopup="dialog"
            onClick={() => setLauncher(true)}
          >
            <LayoutGrid size={18} strokeWidth={1.6} />
            <span className="nav-rail-tip">{t("All apps")}</span>
          </button>
          {/* 设置只从头像菜单进（用户 2026-10-05 要求），图标栏不再放 */}
          <ProfileMenu />
        </nav>
        {showPanel && (
          <aside className="nav-panel" aria-label={t(panelLabel)}>
            <div className="nav-panel-head">
              <span>{t(panelLabel)}</span>
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
      {launcher && (
        <Launcher
          entries={entries}
          pinned={(path) => isPinned(pins, path)}
          onTogglePin={(path) => setPins(togglePin(pins, path))}
          onOpen={(entry) => {
            closeLauncher();
            close();
            navigate(entryTarget(entry, location.pathname));
          }}
          onClose={closeLauncher}
        />
      )}
    </>
  );
}

/** 图标栏里的一个入口。keys 是数量、状态、图标这些标记用哪个功能登记的（合并组用组里第一个有的）。 */
function RailLink({
  to,
  label,
  icon: Icon,
  selected,
  keys,
  extras,
  onClose,
}: {
  to: string;
  label: string;
  icon: LucideIcon;
  selected: boolean;
  keys: { icon?: string; badge?: string; status?: string };
  extras: ReturnType<typeof useNavExtras>;
  onClose: () => void;
}) {
  const t = useT();
  const iconHook = keys.icon ? extras.icons[keys.icon] : undefined;
  const badgeHook = keys.badge ? extras.badges[keys.badge] : undefined;
  const statusHook = keys.status ? extras.statuses[keys.status] : undefined;
  return (
    <NavLink
      to={to}
      aria-label={t(label)}
      onClick={onClose}
      className={`nav-rail-item${selected ? " selected" : ""}`}
    >
      {iconHook ? (
        <NavIconMark hook={iconHook} fallback={Icon} />
      ) : (
        <Icon size={18} strokeWidth={1.6} />
      )}
      {badgeHook ? (
        <NavBadgeMark hook={badgeHook} />
      ) : (
        statusHook && <NavStatusDot hook={statusHook} />
      )}
      <span className="nav-rail-tip">
        {t(label)}
        {statusHook && <NavStatusText hook={statusHook} />}
      </span>
    </NavLink>
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
  // B179：在合并组里时，二级菜单顶部列出组里的功能，下面是当前功能自己的菜单
  const entry = current ? entryOf(current.path) : undefined;
  const visibleMembers = (entry ? (navMerges[entry] ?? []) : []).filter((p) =>
    modules.has(moduleOfPath(p)),
  );
  const merged = visibleMembers.length > 1;
  const Panel = merged
    ? MergedNavPanel
    : current
      ? children[current.path]
      : undefined;
  const panelLabel = merged
    ? (buildEntries(navItems).find((e) => e.path === entry)?.label ?? "")
    : (current?.label ?? "");
  return { current, currentPath, Panel, panelLabel };
}

/** 合并组的二级菜单：组里的功能在上面，当前功能自己的菜单在下面（B179）。 */
function MergedNavPanel({ onNavigate }: { onNavigate: () => void }) {
  const t = useT();
  const location = useLocation();
  const modules = useModules();
  const children = useNavChildren();
  const currentPath = moduleForPath(location.pathname);
  const entry = currentPath ? entryOf(currentPath) : "";
  const members = navItems.filter(
    (n) =>
      (navMerges[entry] ?? []).includes(n.path) &&
      modules.has(moduleOfPath(n.path)),
  );
  const Inner = currentPath ? children[currentPath] : undefined;
  return (
    <>
      <div className="nav-merge">
        <NavPanelGroup>
          {members.map((m) => (
            <NavPanelLink
              key={m.path}
              to={lastPathFor(m.path, location.pathname)}
              icon={m.icon}
              label={t(m.label)}
              active={m.path === currentPath}
              onNavigate={onNavigate}
            />
          ))}
        </NavPanelGroup>
      </div>
      {Inner && <Inner onNavigate={onNavigate} />}
    </>
  );
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
  cyber: "Cyber",
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
