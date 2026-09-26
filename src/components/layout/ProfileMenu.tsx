import type * as Model from "../../types/domain";
import React from "react";
import { Monitor, Moon, Sun, Check, Command, ArrowRight } from "lucide-react";

interface ProfileMenuProps {
  profileClosing: boolean;
  t: Model.Translate;
  setLanguage: Model.Setter<Model.Language>;
  language: Model.Language;
  setProfileOpen: Model.Setter<boolean>;
  themeMode: Model.ThemeMode;
  setThemeMode: Model.Setter<Model.ThemeMode>;
  openSearch: () => void;
}

export default function ProfileMenu({
  profileClosing,
  t,
  setLanguage,
  language,
  setProfileOpen,
  themeMode,
  setThemeMode,
  openSearch,
}: ProfileMenuProps) {
  return (
    <div
      className={"profile-popover" + (profileClosing ? " is-closing" : "")}
      role="menu"
      aria-label={t("Account and settings")}
    >
      <div className="profile-email">
        <span>jo@xcc.ai</span>
        <button
          className="profile-language-button"
          onClick={() => {
            setLanguage(language === "zh" ? "en" : "zh");
            setProfileOpen(false);
          }}
          title={language === "zh" ? "Switch to English" : "切换为中文"}
        >
          {language === "zh" ? "中 / EN" : "EN / 中"}
        </button>
      </div>
      <div className="profile-menu-section">
        <div className="profile-menu-label">{t("Theme")}</div>
        {(
          [
            ["system", "System", Monitor],
            ["dark", "Dark", Moon],
            ["light", "Light", Sun],
          ] as const
        ).map(([mode, label, Icon]) => (
          <button
            className="profile-menu-item"
            role="menuitemradio"
            aria-checked={themeMode === mode}
            key={mode}
            onClick={() => {
              setThemeMode(mode);
              setProfileOpen(false);
            }}
          >
            <Icon size={15} />
            <span>{t(label)}</span>
            {themeMode === mode && <Check size={15} className="menu-check" />}
          </button>
        ))}
      </div>
      <div className="profile-menu-section profile-menu-actions">
        <button
          className="profile-menu-item"
          role="menuitem"
          onClick={() => {
            setProfileOpen(false);
            openSearch();
          }}
        >
          <Command size={15} />
          <span>{t("Commands")}</span>
          <kbd>⌘K</kbd>
        </button>
        <button className="profile-menu-item" role="menuitem" disabled>
          <ArrowRight size={15} />
          <span>{t("Sign out")}</span>
        </button>
      </div>
    </div>
  );
}
