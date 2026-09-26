import { Suspense, type ReactNode } from "react";
import { NavLink, Navigate } from "react-router";
import { WifiOff } from "lucide-react";
import { useServerEvent } from "../../../api/events";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import { applyMetricsEvent, useHost, type HostDetail } from "../api";
import { tabsFor } from "../tabs";

interface HostViewProps {
  hostId: string;
  /** 标签链接的前缀，例如 /servers/abc */
  basePath: string;
  tab?: string;
  /** 页头，拿到机器详情后渲染 */
  heading: (host: HostDetail) => ReactNode;
  /** 标签上方的额外内容（本机页的剪贴板和快捷操作） */
  extra?: (host: HostDetail) => ReactNode;
}

/** 一台机器的详情：页头、离线提示、标签页。服务器和本机共用。 */
export default function HostView({ hostId, basePath, tab, heading, extra }: HostViewProps) {
  const t = useT();
  const language = useLanguage();
  const host = useHost(hostId);
  useServerEvent("host.metrics", applyMetricsEvent);

  if (host.isPending) return <Loading />;
  if (host.isError) return <ErrorState error={host.error} onRetry={() => host.refetch()} />;
  const h = host.data;
  const tabs = tabsFor(h);
  const current = tabs.find((x) => x.id === (tab ?? tabs[0].id));
  if (!current) return <Navigate to={basePath} replace />;
  const Component = current.component;
  return (
    <>
      {heading(h)}
      {!h.online && (
        <div className="servers-offline" role="status">
          <WifiOff size={16} />
          <span>
            {t("This machine is offline.")}{" "}
            {h.lastSeenAt ? `${t("Last seen")} ${relativeTime(h.lastSeenAt, language)}` : t("It has never connected.")}
          </span>
        </div>
      )}
      {extra?.(h)}
      <nav className="xc-tabs">
        {tabs.map((x, i) => (
          <NavLink
            key={x.id}
            to={i === 0 ? basePath : `${basePath}/${x.id}`}
            end
            className={() => (x.id === current.id ? "active" : "")}
          >
            {t(x.label)}
          </NavLink>
        ))}
      </nav>
      <Suspense fallback={<Loading />}>
        <Component key={`${h.id}-${current.id}`} host={h} />
      </Suspense>
    </>
  );
}
