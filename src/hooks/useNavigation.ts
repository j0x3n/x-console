import type { Navigate } from "../types/domain";
import { useState, useEffect } from "react";
import { viewFromPath, pathForView } from "../lib/navigation";

export function useNavigation(onNavigate?: () => void) {
  const [view, setView] = useState(() =>
    viewFromPath(window.location.pathname),
  );
  useEffect(() => {
    const onPopState = () => setView(viewFromPath(window.location.pathname));
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);
  useEffect(() => {
    if (view !== "Crew") return;
    const anchor = window.location.hash.slice(1);
    if (
      !["scout", "scribe", "ledger", "pilot", "echo", "hire"].includes(anchor)
    )
      return;
    window.requestAnimationFrame(() =>
      window.requestAnimationFrame(() => {
        const scroller = document.querySelector(".main-scroll");
        const target = document.getElementById(anchor);
        if (scroller && target)
          scroller.scrollTo({
            top:
              scroller.scrollTop +
              target.getBoundingClientRect().top -
              scroller.getBoundingClientRect().top -
              68,
            behavior: "smooth",
          });
      }),
    );
  }, [view]);
  const navigate: Navigate = (name, anchor) => {
    if (!name) return;
    const nextView = name === "xcc" ? "Deals" : name;
    setView(nextView);
    const nextPath = pathForView(nextView);
    const nextUrl = nextPath + (anchor ? `#${anchor}` : "");
    if (window.location.pathname + window.location.hash !== nextUrl)
      window.history.pushState({}, "", nextUrl);
    onNavigate?.();
    const scroller = document.querySelector(".main-scroll");
    if (!anchor) scroller?.scrollTo({ top: 0, behavior: "instant" });
    else
      window.requestAnimationFrame(() =>
        window.requestAnimationFrame(() => {
          const target = document.getElementById(anchor);
          if (target && scroller)
            scroller.scrollTo({
              top:
                scroller.scrollTop +
                target.getBoundingClientRect().top -
                scroller.getBoundingClientRect().top -
                68,
              behavior: "smooth",
            });
        }),
      );
  };

  return { view, navigate };
}
