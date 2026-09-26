import { useState, useEffect } from "react";

export function usePresence(
  visible: boolean,
  duration = 170,
): [boolean, boolean] {
  const [mounted, setMounted] = useState(visible);
  useEffect(() => {
    if (visible) {
      setMounted(true);
      return;
    }
    const timer = window.setTimeout(() => setMounted(false), duration);
    return () => window.clearTimeout(timer);
  }, [visible, duration]);
  return [visible || mounted, mounted && !visible];
}
