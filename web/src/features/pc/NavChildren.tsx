import { useLocation } from "react-router";
import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useHosts } from "../servers/api";
import { loadSelectedHost } from "./recent";

/** 侧边栏“电脑”下面：每台电脑一行，在线的排前面。有一台以上才有用。 */
export default function PcNavChildren({ onNavigate }: NavChildrenProps) {
  const hosts = useHosts("desktop");
  const location = useLocation();
  const selected = loadSelectedHost();
  const list = [...(hosts.data ?? [])].sort(
    (a, b) =>
      Number(b.online) - Number(a.online) || a.name.localeCompare(b.name),
  );
  return (
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
    />
  );
}
