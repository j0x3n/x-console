import { useParams } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import HostView from "./components/HostView";
import { HostHeadStatus } from "./components/HostCard";

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
            <PageHeading
              title={h.name}
              subtitle={[
                h.hostname,
                h.os && `${h.os}/${h.arch}`,
                h.source === "ssh" ? t("SSH only") : "",
              ]
                .filter(Boolean)
                .join(" · ")}
              aside={<HostHeadStatus host={h} />}
            />
          </>
        )}
      />
    </div>
  );
}
