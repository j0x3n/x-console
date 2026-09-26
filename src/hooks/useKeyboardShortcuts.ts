import type * as Model from "../types/domain";
import { useEffect } from "react";

interface KeyboardShortcutOptions {
  view: string;
  searchOpen: boolean;
  delegateOpen: boolean;
  editItem: Model.Decision | null;
  decisions: Model.Decision[];
  activeDecision: number;
  language: Model.Language;
  setSearchOpen: Model.Setter<boolean>;
  setDelegateOpen: Model.Setter<boolean>;
  setEditItem: Model.Setter<Model.Decision | null>;
  setNotificationsOpen: Model.Setter<boolean>;
  setMobileOpen: Model.Setter<boolean>;
  setActiveDecision: Model.Setter<number>;
  onAction: Model.DecisionHandler;
}

export function useKeyboardShortcuts({
  view,
  searchOpen,
  delegateOpen,
  editItem,
  decisions,
  activeDecision,
  language,
  setSearchOpen,
  setDelegateOpen,
  setEditItem,
  setNotificationsOpen,
  setMobileOpen,
  setActiveDecision,
  onAction,
}: KeyboardShortcutOptions) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.target instanceof HTMLElement)) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (
        e.key.toLowerCase() === "d" &&
        !e.metaKey &&
        !e.ctrlKey &&
        !searchOpen &&
        !delegateOpen &&
        !editItem &&
        !["INPUT", "TEXTAREA"].includes(e.target.tagName)
      ) {
        e.preventDefault();
        setDelegateOpen(true);
      }
      if (e.key === "Escape") {
        setSearchOpen(false);
        setDelegateOpen(false);
        setEditItem(null);
        setNotificationsOpen(false);
        setMobileOpen(false);
      }
      if (
        view === "Today" &&
        !searchOpen &&
        !delegateOpen &&
        !editItem &&
        e.target.tagName !== "INPUT" &&
        e.target.tagName !== "TEXTAREA" &&
        !e.target.closest(
          "button, a, [role='menu'], [role='dialog'], [contenteditable='true']",
        )
      ) {
        if (e.key === "j")
          setActiveDecision((i) => Math.min(i + 1, decisions.length - 1));
        if (e.key === "k") setActiveDecision((i) => Math.max(i - 1, 0));
        if (e.key === "Enter" && decisions[activeDecision])
          onAction(decisions[activeDecision], "approved");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    view,
    searchOpen,
    delegateOpen,
    editItem,
    decisions,
    activeDecision,
    language,
  ]);
}
