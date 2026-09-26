import { useState, useRef, useEffect } from "react";
import type { Notify, ToastNotification } from "../types/domain";

export function useToast() {
  const [toast, setToast] = useState<ToastNotification | null>(null);
  const toastTimer = useRef<number | null>(null);
  const toastSequence = useRef(0);
  const showToast: Notify = (message, undoKey = null) => {
    if (toastTimer.current) window.clearTimeout(toastTimer.current);
    setToast(
      typeof message === "object"
        ? { ...message, id: ++toastSequence.current }
        : { message, undoKey, id: ++toastSequence.current },
    );
    toastTimer.current = window.setTimeout(
      () =>
        setToast((current) => (current ? { ...current, closing: true } : null)),
      5000,
    );
  };
  const hideToast = () => {
    if (toastTimer.current) window.clearTimeout(toastTimer.current);
    setToast((current) => (current ? { ...current, closing: true } : null));
  };
  useEffect(() => {
    if (!toast?.closing) return;
    const id = toast.id;
    const timer = window.setTimeout(
      () => setToast((current) => (current?.id === id ? null : current)),
      170,
    );
    return () => window.clearTimeout(timer);
  }, [toast?.closing, toast?.id]);
  useEffect(
    () => () => {
      if (toastTimer.current) window.clearTimeout(toastTimer.current);
    },
    [],
  );

  return { toast, showToast, hideToast };
}
