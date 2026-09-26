import { useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router";
import { useServerEvents } from "../api/events";
import ElevationDialog from "../auth/ElevationDialog";
import CommandPalette from "../components/command/CommandPalette";
import Sidebar from "../components/layout/Sidebar";
import Topbar from "../components/layout/Topbar";
import Toast from "../components/ui/Toast";
import { useToastStore } from "../hooks/useToast";
import GlobalPanels from "./GlobalPanels";

export default function Layout() {
  useServerEvents();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const toast = useToastStore((s) => s.current);
  const hideToast = useToastStore((s) => s.hide);
  const location = useLocation();
  useEffect(() => setMobileOpen(false), [location.pathname]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
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
          <Outlet />
        </div>
      </main>
      <GlobalPanels />
      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
      />
      <ElevationDialog />
      {toast && <Toast key={toast.id} toast={toast} hideToast={hideToast} />}
    </div>
  );
}
