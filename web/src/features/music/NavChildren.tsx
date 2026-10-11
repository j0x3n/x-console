import { useLocation } from "react-router";
import {
  Clock,
  Disc3,
  Heart,
  ListChecks,
  ListMusic,
  Mic2,
  Music,
} from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { usePending, usePlaylists } from "./api";
import { parseView } from "./MusicPage";

/** 左栏“音乐”的二级菜单（B147）：歌曲、专辑、歌手，我喜欢的、最近播放，播放列表。 */
export default function MusicNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const params = new URLSearchParams(
    location.pathname === "/music" ? location.search : "",
  );
  const atMusic = location.pathname === "/music";
  const view = parseView(params.get("view"));
  const playlist = Number(params.get("playlist")) || null;
  const lists = usePlaylists();
  const pending = usePending();
  const link = (
    to: string,
    icon: typeof Music,
    label: string,
    active: boolean,
  ) => (
    <NavPanelLink
      to={to}
      icon={icon}
      label={t(label)}
      active={active}
      onNavigate={onNavigate}
    />
  );
  return (
    <NavPanelStack>
      <NavPanelGroup>
        {link("/music", Music, "All songs", atMusic && view === "songs")}
        {link(
          "/music?view=albums",
          Disc3,
          "Albums",
          atMusic && view === "albums",
        )}
        {link(
          "/music?view=artists",
          Mic2,
          "Artists",
          atMusic && view === "artists",
        )}
      </NavPanelGroup>
      <NavPanelGroup>
        {link(
          "/music?view=favorites",
          Heart,
          "Favorites",
          atMusic && view === "favorites",
        )}
        {link(
          "/music?view=recent",
          Clock,
          "Recently played",
          atMusic && view === "recent",
        )}
        <NavPanelLink
          to="/music?view=pending"
          icon={ListChecks}
          label={t("To confirm")}
          count={pending.data?.length || null}
          active={atMusic && view === "pending"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Playlists")}>
        {link(
          "/music?view=playlists",
          ListMusic,
          "All playlists",
          atMusic && view === "playlists" && playlist == null,
        )}
        {(lists.data ?? []).slice(0, 20).map((p) => (
          <NavPanelLink
            key={p.id}
            to={`/music?view=playlists&playlist=${p.id}`}
            label={p.name}
            count={p.trackCount}
            active={atMusic && view === "playlists" && playlist === p.id}
            onNavigate={onNavigate}
          />
        ))}
      </NavPanelGroup>
    </NavPanelStack>
  );
}
