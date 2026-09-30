import { useLayoutEffect } from "react";

export function useBrowserViewport() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const viewport = window.visualViewport;
    let frame = 0;
    const update = () => {
      const zoomed = viewport && Math.abs(viewport.scale - 1) > 0.01;
      root.style.setProperty("--xc-window-height", `${window.innerHeight}px`);
      if (!zoomed) {
        root.style.setProperty(
          "--xc-visible-height",
          `${viewport?.height ?? window.innerHeight}px`,
        );
        root.style.setProperty(
          "--xc-visible-top",
          `${viewport?.offsetTop ?? 0}px`,
        );
      }
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("resize", schedule);
    viewport?.addEventListener("resize", schedule);
    viewport?.addEventListener("scroll", schedule);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", schedule);
      viewport?.removeEventListener("resize", schedule);
      viewport?.removeEventListener("scroll", schedule);
      root.style.removeProperty("--xc-window-height");
      root.style.removeProperty("--xc-visible-height");
      root.style.removeProperty("--xc-visible-top");
    };
  }, []);
}
