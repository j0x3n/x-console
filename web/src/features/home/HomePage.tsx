import { useState } from "react";
import { Link } from "react-router";
import { House, Settings, WifiOff } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { isNotConfigured, useHAEvents, useHAStatus, type HAStatus } from "./api";
import EntitiesView from "./components/EntitiesView";
import FavoritesView from "./components/FavoritesView";

type Tab = "favorites" | "all";
const TAB_KEY = "home.tab";

function readTab(): Tab {
  try {
    return localStorage.getItem(TAB_KEY) === "all" ? "all" : "favorites";
  } catch {
    return "favorites";
  }
}

export default function HomePage() {
  const t = useT();
  useHAEvents();
  const status = useHAStatus();
  const [tab, setTabState] = useState<Tab>(readTab);
  const setTab = (next: Tab) => {
    setTabState(next);
    try {
      localStorage.setItem(TAB_KEY, next);
    } catch {
      /* 无痕模式等情况下存不了，忽略 */
    }
  };

  let content;
  if (status.isPending) content = <Loading />;
  else if (status.isError)
    content = isNotConfigured(status.error) ? (
      <SetupGuide />
    ) : (
      <ErrorState error={status.error} onRetry={() => status.refetch()} />
    );
  else if (!status.data.configured) content = <SetupGuide />;
  else
    content = (
      <>
        {!status.data.connected && <OfflineNotice status={status.data} />}
        <nav className="xc-tabs">
          <button
            className={tab === "favorites" ? "active" : ""}
            onClick={() => setTab("favorites")}
          >
            {t("Favorites")}
          </button>
          <button
            className={tab === "all" ? "active" : ""}
            onClick={() => setTab("all")}
          >
            {t("All devices")}
          </button>
        </nav>
        {tab === "favorites" ? (
          <FavoritesView onBrowse={() => setTab("all")} />
        ) : (
          <EntitiesView />
        )}
      </>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Smart home")}
        aside={status.data?.configured && <StatusBadge status={status.data} />}
      />
      {content}
    </div>
  );
}

function StatusBadge({ status }: { status: HAStatus }) {
  const t = useT();
  return status.connected ? (
    <span className="xc-badge ok" title={`Home Assistant ${status.version ?? ""}`}>
      <span className="xc-dot ok" /> {t("Connected")}
    </span>
  ) : (
    <span className="xc-badge warn" title={status.error}>
      <span className="xc-dot warn" /> {t("Not connected")}
    </span>
  );
}

function OfflineNotice({ status }: { status: HAStatus }) {
  const t = useT();
  return (
    <div className="xc-card home-offline" role="status">
      <WifiOff size={16} aria-hidden />
      <div>
        <strong>{t("Home Assistant is not connected")}</strong>
        <p className="xc-muted">
          {status.error ?? "正在重新连接。"} 下面显示的是最后一次收到的状态。
        </p>
      </div>
      <Link className="xc-btn small" to="/settings/homeassistant">
        {t("Settings")}
      </Link>
    </div>
  );
}

function SetupGuide() {
  const t = useT();
  return (
    <EmptyState
      title={t("Connect Home Assistant")}
      icon={<House size={28} />}
    >
      <span>填好 Home Assistant 的地址和长期访问令牌，就能在这里控制家里的设备。</span>
      <Link className="xc-btn small primary" to="/settings/homeassistant">
        <Settings size={14} /> {t("Go to settings")}
      </Link>
    </EmptyState>
  );
}
