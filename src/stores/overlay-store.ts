import { create } from "zustand";
import { defaultDraft, defaultDraftZh } from "../data/drafts";
import { resolveUpdate } from "./update";
import type { Decision, Language, Setter, OpenDelegate } from "../types/domain";

export interface OverlayState {
  searchOpen: boolean;
  notificationsOpen: boolean;
  delegateOpen: boolean;
  delegateAssignee: string;
  mobileOpen: boolean;
  editItem: Decision | null;
  drafts: Record<Language, string>;
  draftCustomized: boolean;
  taskText: string;
  setSearchOpen: Setter<boolean>;
  setNotificationsOpen: Setter<boolean>;
  setDelegateOpen: Setter<boolean>;
  setMobileOpen: Setter<boolean>;
  setEditItem: Setter<Decision | null>;
  setDrafts: Setter<Record<Language, string>>;
  setDraftCustomized: Setter<boolean>;
  setTaskText: Setter<string>;
  openDelegation: OpenDelegate;
}
export const useOverlayStore = create<OverlayState>()((set) => ({
  searchOpen: false,
  notificationsOpen: false,
  delegateOpen: false,
  delegateAssignee: "Scout",
  mobileOpen: false,
  editItem: null,
  drafts: { en: defaultDraft, zh: defaultDraftZh },
  draftCustomized: false,
  taskText: "",
  setSearchOpen: (update) =>
    set((state) => ({ searchOpen: resolveUpdate(update, state.searchOpen) })),
  setNotificationsOpen: (update) =>
    set((state) => ({
      notificationsOpen: resolveUpdate(update, state.notificationsOpen),
    })),
  setDelegateOpen: (update) =>
    set((state) => ({
      delegateOpen: resolveUpdate(update, state.delegateOpen),
    })),
  setMobileOpen: (update) =>
    set((state) => ({ mobileOpen: resolveUpdate(update, state.mobileOpen) })),
  setEditItem: (update) =>
    set((state) => ({ editItem: resolveUpdate(update, state.editItem) })),
  setDrafts: (update) =>
    set((state) => ({ drafts: resolveUpdate(update, state.drafts) })),
  setDraftCustomized: (update) =>
    set((state) => ({
      draftCustomized: resolveUpdate(update, state.draftCustomized),
    })),
  setTaskText: (update) =>
    set((state) => ({ taskText: resolveUpdate(update, state.taskText) })),
  openDelegation: (name) =>
    set({
      delegateAssignee: typeof name === "string" ? name : "Scout",
      delegateOpen: true,
    }),
}));
