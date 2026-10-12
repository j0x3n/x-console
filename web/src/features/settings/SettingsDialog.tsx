import { Suspense, useEffect, useRef } from "react";
import { useLocation } from "react-router";
import { ChevronLeft, X } from "lucide-react";
import ErrorBoundary from "../../components/ui/ErrorBoundary";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { settingsGroups, settingsTabs } from "./tabs";
import VersionLine from "../maintenance/VersionLine";
import "../maintenance/maintenance.css";

interface Props {
  tab: string; // 空或认不出时：桌面显示第一项，手机先显示菜单
  onSelect: (id: string) => void;
  onClose: () => void;
}

/** 设置弹窗（B180）：左边分组菜单，右边是选中的那一页。 */
export default function SettingsDialog({ tab, onSelect, onClose }: Props) {
  const t = useT();
  const { hash } = useLocation();
  const current = settingsTabs.find((item) => item.id === tab);
  const shown = current ?? settingsTabs[0];
  const Component = shown.component;
  const bodyRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  // Esc 关闭。设置里又开了别的弹窗时，Esc 只关上面那个。
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (document.querySelectorAll(".modal-backdrop").length > 1) return;
      onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    dialogRef.current?.focus();
  }, []);

  // 换页回到顶部。地址带 #xxx 时等内容加载出来再滚过去。
  useEffect(() => {
    const body = bodyRef.current;
    if (!body) return;
    body.scrollTop = 0;
    if (!hash) return;
    let tries = 0;
    const timer = window.setInterval(() => {
      const target = body.querySelector(hash.replace(/[^#\w-]/g, ""));
      if (target || ++tries > 20) window.clearInterval(timer);
      target?.scrollIntoView?.({ block: "start" });
    }, 150);
    return () => window.clearInterval(timer);
  }, [shown.id, hash]);

  return (
    <div
      className="modal-backdrop settings-backdrop"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <div
        ref={dialogRef}
        className={`settings-dialog${current ? "" : " is-menu"}`}
        role="dialog"
        aria-modal="true"
        aria-label={t("Settings")}
        tabIndex={-1}
      >
        <nav className="settings-nav" aria-label={t("Settings")}>
          <div className="settings-nav-head">
            <h2>{t("Settings")}</h2>
            <button
              type="button"
              className="xc-btn ghost small settings-icon-btn settings-close"
              onClick={onClose}
              aria-label={t("Close settings")}
              title={t("Close settings")}
            >
              <X size={16} />
            </button>
          </div>
          <div className="settings-nav-list">
            {settingsGroups.map((group) => (
              <div key={group.id} className="settings-nav-group">
                <span className="settings-nav-label">{t(group.label)}</span>
                {settingsTabs
                  .filter((item) => item.group === group.id)
                  .map((item) => {
                    const Icon = item.icon;
                    const active = item.id === shown.id;
                    return (
                      <button
                        key={item.id}
                        type="button"
                        className={`settings-nav-item${active ? " active" : ""}`}
                        aria-current={active ? "page" : undefined}
                        onClick={() => onSelect(item.id)}
                      >
                        <Icon size={15} />
                        <span>{t(item.label)}</span>
                      </button>
                    );
                  })}
              </div>
            ))}
          </div>
          <VersionLine />
        </nav>
        <section className="settings-main">
          <header className="settings-main-head">
            <button
              type="button"
              className="xc-btn ghost small settings-icon-btn settings-back"
              onClick={() => onSelect("")}
              aria-label={t("Back")}
              title={t("Back")}
            >
              <ChevronLeft size={18} />
            </button>
            <h3>{t(shown.label)}</h3>
            <button
              type="button"
              className="xc-btn ghost small settings-icon-btn"
              onClick={onClose}
              aria-label={t("Close settings")}
              title={t("Close settings")}
            >
              <X size={16} />
            </button>
          </header>
          <div className="settings-main-body" ref={bodyRef}>
            <ErrorBoundary key={shown.id}>
              <Suspense fallback={<Loading />}>
                <Component />
              </Suspense>
            </ErrorBoundary>
          </div>
        </section>
      </div>
    </div>
  );
}
