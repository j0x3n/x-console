import type * as Model from "../../types/domain";
import React from "react";
import {
  Menu,
  ChevronRight,
  ChevronUp,
  ChevronDown,
  Plus,
  SquarePen,
  Bell,
  Command,
} from "lucide-react";
import { companyRecords, peopleRecords } from "../../data/workspace";
import NotificationsPopover from "./NotificationsPopover";

interface TopbarProps {
  t: Model.Translate;
  setMobileOpen: Model.Setter<boolean>;
  view: string;
  navigate: Model.Navigate;
  language: Model.Language;
  setHireRequest: Model.Setter<number>;
  openDelegation: Model.OpenDelegate;
  notificationsRef: React.RefObject<HTMLDivElement | null>;
  notificationsOpen: boolean;
  setNotificationsOpen: Model.Setter<boolean>;
  decisions: Model.Decision[];
  notificationsMounted: boolean;
  notificationsClosing: boolean;
  setSearchOpen: Model.Setter<boolean>;
}

export default function Topbar({
  t,
  setMobileOpen,
  view,
  navigate,
  language,
  setHireRequest,
  openDelegation,
  notificationsRef,
  notificationsOpen,
  setNotificationsOpen,
  decisions,
  notificationsMounted,
  notificationsClosing,
  setSearchOpen,
}: TopbarProps) {
  return (
    <header className="topbar">
      <nav className="breadcrumbs" aria-label={t("Breadcrumb")}>
        <button
          className="mobile-menu icon-button"
          onClick={() => setMobileOpen(true)}
          aria-label={t("Open navigation")}
        >
          <Menu size={18} />
        </button>
        {["Companies", "People", "Deals"].includes(view) ||
        companyRecords.some((c) => c.name === view) ||
        peopleRecords.some((p) => p.name === view) ? (
          <>
            <button onClick={() => navigate("Companies")}>
              {t("Records")}
            </button>
            <ChevronRight size={13} aria-hidden="true" />
            {companyRecords.some((c) => c.name === view) ||
            peopleRecords.some((p) => p.name === view) ? (
              <>
                <button
                  onClick={() =>
                    navigate(
                      companyRecords.some((c) => c.name === view)
                        ? "Companies"
                        : "People",
                    )
                  }
                >
                  {t(
                    companyRecords.some((c) => c.name === view)
                      ? "Companies"
                      : "People",
                  )}
                </button>
                <ChevronRight size={13} aria-hidden="true" />
                <span aria-current="page">{view}</span>
              </>
            ) : (
              <span aria-current="page">{t(view)}</span>
            )}
          </>
        ) : (
          <span aria-current="page">{t(view)}</span>
        )}
      </nav>
      <div className="header-actions">
        {companyRecords.some((item) => item.name === view) ? (
          <div className="account-pager">
            <span>
              {companyRecords.findIndex((item) => item.name === view) + 1}{" "}
              {language === "zh" ? "/" : "of"} {companyRecords.length}
            </span>
            <button
              className="icon-button"
              aria-label={t("Previous company")}
              disabled={companyRecords[0].name === view}
              onClick={() =>
                navigate(
                  companyRecords[
                    companyRecords.findIndex((item) => item.name === view) - 1
                  ]?.name,
                )
              }
            >
              <ChevronUp size={15} />
            </button>
            <button
              className="icon-button"
              aria-label={t("Next company")}
              disabled={companyRecords[companyRecords.length - 1].name === view}
              onClick={() =>
                navigate(
                  companyRecords[
                    companyRecords.findIndex((item) => item.name === view) + 1
                  ]?.name,
                )
              }
            >
              <ChevronDown size={15} />
            </button>
          </div>
        ) : view === "Crew" ? (
          <button
            className="delegate-button"
            onClick={() => setHireRequest((value) => value + 1)}
          >
            <Plus size={16} strokeWidth={1.5} /> {t("Hire an agent")}
          </button>
        ) : (
          <button className="delegate-button" onClick={() => openDelegation()}>
            <SquarePen size={16} strokeWidth={1.5} /> {t("Delegate")}{" "}
            <kbd>D</kbd>
          </button>
        )}
        <div className="notifications-wrap" ref={notificationsRef}>
          <button
            className="icon-button notification-button"
            title={t("Notifications")}
            aria-label={t("Notifications")}
            aria-expanded={notificationsOpen}
            aria-haspopup="dialog"
            onClick={() => setNotificationsOpen(!notificationsOpen)}
          >
            <Bell size={16} />
            {decisions.length > 0 && <i />}
          </button>
          {notificationsMounted && (
            <NotificationsPopover
              notificationsClosing={notificationsClosing}
              t={t}
              language={language}
              decisions={decisions}
              navigate={navigate}
              setNotificationsOpen={setNotificationsOpen}
            />
          )}
        </div>
        <button
          className="command-button"
          onClick={() => setSearchOpen(true)}
          title={t("Open command palette")}
          aria-label={t("Open command palette")}
        >
          <Command size={13} />
          <span>K</span>
        </button>
      </div>
    </header>
  );
}
