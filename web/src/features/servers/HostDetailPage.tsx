import { Link, useParams } from "react-router";
import { ArrowLeft } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import HostView from "./components/HostView";
import { HostStatus } from "./components/HostCard";

export default function HostDetailPage() {
  const t = useT();
  const { hostId = "", tab } = useParams();
  return (
    <div className="xc-page">
      <HostView
        hostId={hostId}
        tab={tab}
        basePath={`/servers/${encodeURIComponent(hostId)}`}
        heading={(h) => (
          <>
            <Link to="/servers" className="servers-back">
              <ArrowLeft size={14} /> {t("Servers")}
            </Link>
            <PageHeading
              title={h.name}
              subtitle={[h.hostname, h.os && `${h.os}/${h.arch}`, h.source === "ssh" ? t("SSH only") : ""].filter(Boolean).join(" · ")}
              aside={<HostStatus host={h} />}
            />
          </>
        )}
      />
    </div>
  );
}
