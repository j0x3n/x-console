import { useState } from "react";
import { Link, useParams } from "react-router";
import { Monitor } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useHosts } from "../servers/api";
import { HostStatus } from "../servers/components/HostCard";
import HostView from "../servers/components/HostView";
import { pickDesktop } from "../servers/lib";
import { ClipboardCard, QuickActionsCard } from "./QuickCards";
import { loadSelectedHost, saveSelectedHost } from "./recent";

/** 本机：只看 kind=desktop 的机器，通常只有一台，直接进详情。 */
export default function PcPage() {
  const t = useT();
  const { tab } = useParams();
  const hosts = useHosts("desktop");
  const [wanted, setWanted] = useState(loadSelectedHost);

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
        <PageHeading title={t("This PC")} />
        <EmptyState title={t("No PC paired yet")} icon={<Monitor size={28} />}>
          <span>
            {t("Install the agent on your Windows PC and pair it as a PC.")}
          </span>
          <Link className="xc-btn primary" to="/settings/devices">
            {t("Pair a device")}
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
            title={t("This PC")}
            subtitle={[h.name, h.hostname, h.os].filter(Boolean).join(" · ")}
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
                <HostStatus host={h} />
              </>
            }
          />
        )}
        extra={(h) => (
          <div className="pc-quick">
            <ClipboardCard host={h} />
            <QuickActionsCard host={h} />
          </div>
        )}
      />
    </div>
  );
}
