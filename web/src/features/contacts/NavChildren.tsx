import { useSearchParams } from "react-router";
import { Archive, Users, CalendarDays, Phone } from "lucide-react";
import {
  NavPanelStack,
  NavPanelGroup,
  NavPanelLink,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { GROUPS, GROUP_LABELS, GROUP_ICONS } from "./format";
import { useContacts } from "./api";

export default function NavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const [params] = useSearchParams();
  const archived = params.get("archived") === "1";
  const view = params.get("view") ?? "all";
  const category = params.get("group") ?? "";
  const list = useContacts(archived);
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/contacts"
          icon={Users}
          label={t("All contacts")}
          active={!archived && !category && view === "all"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/contacts?view=soon"
          icon={CalendarDays}
          label={t("Dates coming up")}
          active={!archived && !category && view === "soon"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/contacts?view=overdue"
          icon={Phone}
          label={t("Time to get in touch")}
          active={!archived && !category && view === "overdue"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/contacts?archived=1"
          icon={Archive}
          label={t("Archived")}
          active={archived && !category}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Group")}>
        {GROUPS.map((value) => (
          <NavPanelLink
            key={value}
            to={`/contacts?group=${value}${archived ? "&archived=1" : ""}`}
            icon={GROUP_ICONS[value]}
            label={t(GROUP_LABELS[value])}
            count={
              list.data?.items.filter(
                (item) => item.group === value && item.archived === archived,
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
