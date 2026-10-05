import { useLocation } from "react-router";
import { Settings2 } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useHostOrder, useHosts } from "../servers/api";
import { loadSelectedHost } from "./recent";

/** 左栏“电脑”的二级菜单（B102）：每台电脑一行，按排好的顺序（B82），可以拖动；下面是设备管理。 */
export default function PcNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const hosts = useHosts("desktop");
  const location = useLocation();
  const selected = loadSelectedHost();
  const order = useHostOrder("desktop");
  const list = hosts.data ?? [];
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavChildLinks
          links={list.map((h) => ({
            key: h.id,
            to: `/pc?host=${encodeURIComponent(h.id)}`,
            label: h.name,
            mark: <i className={`xc-dot ${h.online ? "ok" : ""}`} />,
            active: location.pathname.startsWith("/pc") && selected === h.id,
          }))}
          allTo="/settings/devices"
          loading={hosts.isPending}
          error={hosts.isError}
          empty="还没有电脑"
          onNavigate={onNavigate}
          onReorder={(keys) => order.mutate(keys.map(String))}
        />
      </NavPanelGroup>
      <NavPanelGroup>
        <NavPanelLink
          to="/settings/devices"
          icon={Settings2}
          label={t("Manage devices")}
          active={false}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
