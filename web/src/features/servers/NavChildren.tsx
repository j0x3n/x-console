import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useHostOrder, useHosts } from "./api";
import CountryFlag from "./components/CountryFlag";

/**
 * 侧边栏“服务器”下面：和服务器页一样的顺序（B82），可以拖动排序。
 * 离线的显示灰点。
 */
export default function ServersNavChildren({ onNavigate }: NavChildrenProps) {
  const hosts = useHosts("server");
  const order = useHostOrder("server");
  const list = hosts.data ?? [];
  return (
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
  );
}
