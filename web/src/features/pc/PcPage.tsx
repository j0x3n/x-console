import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { Monitor } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useHosts } from "../servers/api";
import { HostHeadStatus } from "../servers/components/HostCard";
import HostView from "../servers/components/HostView";
import { pickDesktop } from "../servers/lib";
import { PcQuickBar } from "./QuickCards";
import { loadSelectedHost, saveSelectedHost } from "./recent";

/** 电脑（原“本机”）：只看 kind=desktop 的机器，通常只有一台，直接进详情。 */
export default function PcPage() {
  const t = useT();
  const { tab } = useParams();
  const hosts = useHosts("desktop");
  const [wanted, setWanted] = useState(loadSelectedHost);
  // 左栏二级菜单点了某台电脑时带 ?host=，以它为准并记住
  const [params, setParams] = useSearchParams();
  const fromNav = params.get("host");
  useEffect(() => {
    if (!fromNav) return;
    setWanted(fromNav);
    saveSelectedHost(fromNav);
    const next = new URLSearchParams(params);
    next.delete("host");
    setParams(next, { replace: true });
  }, [fromNav, params, setParams]);

  if (hosts.isPending)
    return (
      <div className="xc-page">
        <Loading />
      </div>
    );
  if (hosts.isError)
    return (
      <div className="xc-page">
        <ErrorState error={hosts.error} onRetry={() => hosts.refetch()} />
      </div>
    );
  const host = pickDesktop(hosts.data, wanted);
  if (!host)
    return (
      <div className="xc-page">
        <PageHeading title={t("Computer")} />
        <EmptyState title={t("No PC paired yet")} icon={<Monitor size={28} />}>
          <span>
            {t("Run one command on your Windows computer and it connects.")}
          </span>
          <Link className="xc-btn primary" to="/settings/devices?add=desktop">
            {t("Add a computer")}
          </Link>
        </EmptyState>
      </div>
    );
  const choose = (id: string) => {
    setWanted(id);
    saveSelectedHost(id);
  };
  return (
    <div className="xc-page">
      <HostView
        hostId={host.id}
        tab={tab}
        basePath="/pc"
        heading={(h) => (
          <PageHeading
            title={h.name}
            meta={<HostHeadStatus host={h} />}
            aside={
              <>
                {hosts.data.length > 1 && (
                  <select
                    className="xc-select pc-select"
                    value={host.id}
                    onChange={(e) => choose(e.target.value)}
                    aria-label={t("Choose a PC")}
                  >
                    {hosts.data.map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                  </select>
                )}
                <PcQuickBar host={h} />
              </>
            }
          />
        )}
      />
    </div>
  );
}
