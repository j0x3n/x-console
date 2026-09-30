import { Suspense, useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router";
import { useServerEvents } from "../api/events";
import ElevationDialog from "../auth/ElevationDialog";
import { ConfirmHost } from "../components/ui/ConfirmDialog";
import CommandPalette from "../components/command/CommandPalette";
import Sidebar from "../components/layout/Sidebar";
import Topbar from "../components/layout/Topbar";
import { Loading } from "../components/ui/States";
import ErrorBoundary from "../components/ui/ErrorBoundary";
import SafariProbe from "../components/layout/SafariProbe";
import GlobalPanels from "./GlobalPanels";
import { useSidebar } from "../stores/sidebar";
import { usePreferencesSync } from "../hooks/usePreferencesSync";

export default function Layout() {
  useServerEvents();
  usePreferencesSync();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const location = useLocation();
  useEffect(() => {
    setMobileOpen(false);
    if (window.matchMedia("(max-width: 720px)").matches) window.scrollTo(0, 0);
  }, [location.pathname]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "b") {
        event.preventDefault();
        useSidebar.getState().toggle();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);
  return (
    <div className="app-shell">
      <Sidebar
        mobileOpen={mobileOpen}
        setMobileOpen={setMobileOpen}
        openPalette={() => setPaletteOpen(true)}
      />
      <main className="main-panel" id="main">
        <div className="main-scroll">
          <Topbar
            openMobile={() => setMobileOpen(true)}
            openPalette={() => setPaletteOpen(true)}
          />
          <SafariProbe />
          {/* 页面按需加载（B6），加载时显示转圈 */}
          <ErrorBoundary key={location.pathname}>
            <Suspense fallback={<Loading />}>
              <Outlet />
            </Suspense>
          </ErrorBoundary>
        </div>
      </main>
      <GlobalPanels />
      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
      />
      <ElevationDialog />
      <ConfirmHost />
    </div>
  );
}
