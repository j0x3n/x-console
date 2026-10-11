import { useSearchParams } from "react-router";
import {
  Archive,
  KeyRound,
  TriangleAlert,
  Clock,
  RefreshCw,
} from "lucide-react";
import {
  NavPanelStack,
  NavPanelGroup,
  NavPanelLink,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { KINDS, KIND_LABELS, KIND_ICONS } from "./format";
import { useCredentials } from "./api";

export default function NavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const [params] = useSearchParams();
  const archived = params.get("archived") === "1";
  const view = params.get("view") ?? "all";
  const category = params.get("kind") ?? "";
  const list = useCredentials(archived);
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/credentials"
          icon={KeyRound}
          label={t("All credentials")}
          active={!archived && !category && view === "all"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/credentials?view=expired"
          icon={TriangleAlert}
          label={t("Expired")}
          active={!archived && !category && view === "expired"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/credentials?view=soon"
          icon={Clock}
          label={t("Expiring soon")}
          active={!archived && !category && view === "soon"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/credentials?view=stale"
          icon={RefreshCw}
          label={t("Not rotated in time")}
          active={!archived && !category && view === "stale"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/credentials?archived=1"
          icon={Archive}
          label={t("Archived")}
          active={archived && !category}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Type")}>
        {KINDS.map((value) => (
          <NavPanelLink
            key={value}
            to={`/credentials?kind=${value}${archived ? "&archived=1" : ""}`}
            icon={KIND_ICONS[value]}
            label={t(KIND_LABELS[value])}
            count={
              list.data?.items.filter(
                (item) => item.kind === value && item.archived === archived,
              ).length
            }
            active={category === value}
            onNavigate={onNavigate}
          />
        ))}
      </NavPanelGroup>
    </NavPanelStack>
  );
}
