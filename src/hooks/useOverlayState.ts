import { useRef, useEffect } from "react";
import { useOverlayStore } from "../stores/overlay-store";
import { usePresence } from "./usePresence";

export function useOverlayState() {
  const state = useOverlayStore();
  const {
    searchOpen,
    notificationsOpen,
    delegateOpen,
    editItem,
    setNotificationsOpen,
  } = state;
  const [searchMounted, searchClosing] = usePresence(searchOpen);
  const [notificationsMounted, notificationsClosing] =
    usePresence(notificationsOpen);
  const [delegateMounted, delegateClosing] = usePresence(delegateOpen);
  const [editMounted, editClosing] = usePresence(Boolean(editItem));
  const notificationsRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!notificationsOpen) return;
    const onOutside = (event: PointerEvent) => {
      if (
        event.target instanceof Node &&
        !notificationsRef.current?.contains(event.target)
      )
        setNotificationsOpen(false);
    };
    document.addEventListener("pointerdown", onOutside);
    return () => document.removeEventListener("pointerdown", onOutside);
  }, [notificationsOpen, setNotificationsOpen]);
  return {
    ...state,
    searchMounted,
    searchClosing,
    notificationsMounted,
    notificationsClosing,
    notificationsRef,
    delegateMounted,
    delegateClosing,
    editMounted,
    editClosing,
  };
}
