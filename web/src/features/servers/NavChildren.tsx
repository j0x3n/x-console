import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useHosts } from "./api";

/** 侧边栏“服务器”下面：在线的排前面。 */
export default function ServersNavChildren({ onNavigate }: NavChildrenProps) {
  const hosts = useHosts("server");
  const list = [...(hosts.data ?? [])].sort(
    (a, b) =>
      Number(b.online) - Number(a.online) || a.name.localeCompare(b.name),
  );
  return (
    <NavChildLinks
      links={list.map((h) => ({
        key: h.id,
        to: `/servers/${h.id}`,
        label: h.name,
        mark: <i className={`xc-dot ${h.online ? "ok" : ""}`} />,
      }))}
      allTo="/servers"
      loading={hosts.isPending}
      error={hosts.isError}
      empty="还没有服务器"
      onNavigate={onNavigate}
    />
  );
}
