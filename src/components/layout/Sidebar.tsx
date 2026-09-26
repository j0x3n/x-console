import type * as Model from "../../types/domain";
import React, { useState, useRef, useEffect } from "react";
import { useT } from "../../contexts/LanguageContext";
import { usePresence } from "../../hooks/usePresence";
import {
  ChevronDown,
  PanelLeftClose,
  Search,
  Sunrise,
  Table2,
  SquareKanban,
  Shapes,
  Plus,
  Settings2,
} from "lucide-react";
import { agents } from "../../data/catalogs";
import AgentBadge from "../ui/AgentBadge";
import { companies } from "../../data/dashboard";
import CompanyBadge from "../ui/CompanyBadge";
import ProfileMenu from "./ProfileMenu";

interface SidebarProps {
  view: string;
  decisionCount: number;
  navigate: Model.Navigate;
  openSearch: () => void;
  openDelegate: Model.OpenDelegate;
  openHire: () => void;
  mobileOpen: boolean;
  setMobileOpen: Model.Setter<boolean>;
  language: Model.Language;
  setLanguage: Model.Setter<Model.Language>;
  themeMode: Model.ThemeMode;
  setThemeMode: Model.Setter<Model.ThemeMode>;
}

export default function Sidebar({
  view,
  decisionCount,
  navigate,
  openSearch,
  openDelegate,
  openHire,
  mobileOpen,
  setMobileOpen,
  language,
  setLanguage,
  themeMode,
  setThemeMode,
}: SidebarProps) {
  const t = useT();
  const [recordsOpen, setRecordsOpen] = useState(true);
  const [profileOpen, setProfileOpen] = useState(false);
  const [profileMounted, profileClosing] = usePresence(profileOpen);
  const profileMenuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!profileOpen) return;
    const closeOnOutside = (event: PointerEvent) => {
      if (!profileMenuRef.current?.contains(event.target as Node))
        setProfileOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setProfileOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutside);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutside);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [profileOpen]);
  const nav: Model.Navigate = (name, anchor) => {
    navigate(name, anchor);
    setMobileOpen(false);
    setProfileOpen(false);
  };
  return (
    <>
      {mobileOpen && (
        <button
          className="mobile-scrim"
          aria-label={t("Close navigation")}
          onClick={() => setMobileOpen(false)}
        />
      )}
      <nav
        className={`sidebar ${mobileOpen ? "open" : ""}`}
        aria-label={t("Main")}
      >
        <div className="workspace-switch">
          <span className="brand-mark">X</span>
          <span>X Console</span>
          <ChevronDown size={13} />
          <button
            className="icon-button sidebar-collapse"
            title={t("Close navigation")}
            onClick={() => setMobileOpen(false)}
          >
            <PanelLeftClose size={15} />
          </button>
        </div>
        <button className="sidebar-search" onClick={openSearch}>
          <Search size={15} />
          <span>{t("Search or ask...")}</span>
          <kbd>⌘K</kbd>
        </button>
        <div className="nav-groups">
          <button
            className={`nav-item ${view === "Today" ? "selected" : ""}`}
            onClick={() => nav("Today")}
          >
            <Sunrise size={17} strokeWidth={1.5} />
            <span>{t("Today")}</span>
            <em>{decisionCount}</em>
          </button>
          <button
            className="nav-item nav-parent"
            onClick={() => setRecordsOpen(!recordsOpen)}
          >
            <Table2 size={17} strokeWidth={1.5} />
            <span>{t("Records")}</span>
            <ChevronDown size={14} className={recordsOpen ? "" : "rotated"} />
          </button>
          {recordsOpen && (
            <div className="nav-children">
              {[
                ["Companies", "24"],
                ["People", "34"],
                ["Deals", "38"],
              ].map(([name, count]) => (
                <button
                  className={`nav-item ${view === name ? "selected" : ""}`}
                  key={name}
                  onClick={() => nav(name)}
                >
                  <span>{t(name)}</span>
                  <em>{count}</em>
                </button>
              ))}
            </div>
          )}
          <button
            className={`nav-item ${view === "Work" ? "selected" : ""}`}
            onClick={() => nav("Work")}
          >
            <SquareKanban size={17} strokeWidth={1.5} />
            <span>{t("Work")}</span>
          </button>
          <button
            className={`nav-item ${view === "Crew" ? "selected" : ""}`}
            onClick={() => nav("Crew")}
          >
            <Shapes size={17} strokeWidth={1.5} />
            <span>{t("Crew")}</span>
          </button>
          <div className="side-label">
            <span>{t("Crew")}</span>
            <button
              title={t("Hire an agent")}
              onClick={() => nav("Crew", "hire")}
            >
              <Plus size={14} />
            </button>
          </div>
          {agents.map((agent) => (
            <button
              className="nav-item agent-nav"
              key={agent.name}
              onClick={() => nav("Crew", agent.name.toLowerCase())}
            >
              <AgentBadge name={agent.name} size="small" />
              <span>{agent.name}</span>
              <span
                className={`status-dot ${agent.status === "Running" ? "running" : ""}`}
              />
            </button>
          ))}
          <div className="side-label favorites-label">{t("Favorites")}</div>
          {companies.slice(0, 3).map((company) => (
            <button
              className="nav-item favorite-nav"
              key={company.name}
              onClick={() => nav(company.name)}
            >
              <CompanyBadge initials={company.initials} color={company.color} />
              <span>{company.name}</span>
            </button>
          ))}
        </div>
        <div className="profile-menu-wrap" ref={profileMenuRef}>
          {profileMounted && (
            <ProfileMenu
              profileClosing={profileClosing}
              t={t}
              setLanguage={setLanguage}
              language={language}
              setProfileOpen={setProfileOpen}
              themeMode={themeMode}
              setThemeMode={setThemeMode}
              openSearch={openSearch}
            />
          )}
          <button
            className="profile"
            aria-haspopup="menu"
            aria-expanded={profileOpen}
            onClick={() => setProfileOpen((open) => !open)}
          >
            <span className="profile-avatar">JO</span>
            <span>
              <b>jo</b>
              <small>{t("Head of Revenue")}</small>
            </span>
            <Settings2 size={15} />
          </button>
        </div>
      </nav>
    </>
  );
}
