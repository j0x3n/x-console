import { useLocation } from "react-router";
import { Server } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useHostOrder, useHosts } from "./api";
import CountryFlag from "./components/CountryFlag";

/**
 * 左栏“服务器”的二级菜单（B102）：全部服务器，下面每台一行，和服务器页一样的顺序（B82），可以拖动排序。
 * 离线的显示灰点。
 */
export default function ServersNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const { pathname } = useLocation();
  const hosts = useHosts("server");
  const order = useHostOrder("server");
  const list = hosts.data ?? [];
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/servers"
          icon={Server}
          label={t("All servers")}
          count={hosts.data ? list.length : null}
          active={pathname === "/servers"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Machines")}>
        <NavChildLinks
          links={list.map((h) => ({
            key: h.id,
            to: `/servers/${h.id}`,
            label: h.name,
            mark: (
              <>
                <i className={`xc-dot ${h.online ? "ok" : ""}`} />
                {h.country && <CountryFlag country={h.country} />}
              </>
            ),
          }))}
          allTo="/servers"
          loading={hosts.isPending}
          error={hosts.isError}
          empty="还没有服务器"
          onNavigate={onNavigate}
          onReorder={(keys) => order.mutate(keys.map(String))}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
