import { useParams } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import HostView from "./components/HostView";
import { HostHeadStatus } from "./components/HostCard";
import HostHeadExtras from "./components/HostHeadExtras";

export default function HostDetailPage() {
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
              meta={<HostHeadStatus host={h} />}
              aside={<HostHeadExtras host={h} />}
            />
          </>
        )}
      />
    </div>
  );
}
