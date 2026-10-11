import { Heart, Pause, Play } from "lucide-react";
import type { ReactNode } from "react";
import MoreMenu, { type MoreMenuItem } from "../../components/ui/MoreMenu";
import { useT } from "../../contexts/LanguageContext";
import type { Track } from "./api";
import { TrackCover } from "./Cover";
import { formatClock } from "./lyrics";
import { usePlayer } from "./player";
import { useTrackMenu } from "./trackActions";

/*
 * 歌曲列表（B147）：封面、歌名和歌手、专辑、时长、收藏、更多菜单。
 * 桌面双击一行播放，触屏单击播放；封面上的播放按钮两种都能用。
 */
export default function TrackList({
  tracks,
  onPlay,
  extra,
  footer,
}: {
  tracks: Track[];
  /** 播放这一首。通常是“用 tracks 做队列，从这首开始”。 */
  onPlay: (track: Track, index: number) => void;
  /** 这个列表特有的菜单项，比如“从播放列表移除”。 */
  extra?: (track: Track, index: number) => MoreMenuItem[];
  /** 列表下面的内容，比如“加载更多”。 */
  footer?: ReactNode;
}) {
  const t = useT();
  const currentId = usePlayer((s) => s.currentId);
  const playing = usePlayer((s) => s.playing);
  const toggle = usePlayer((s) => s.toggle);
  const { items, toggleFavorite } = useTrackMenu();
  const coarse =
    typeof window !== "undefined" &&
    window.matchMedia?.("(pointer: coarse)").matches === true;

  return (
    <div className="xc-card music-tracks" role="list">
      {tracks.map((track, index) => {
        const active = track.id === currentId;
        const play = () => (active ? toggle() : onPlay(track, index));
        return (
          <div
            key={track.id}
            role="listitem"
            className={`music-row${active ? " active" : ""}`}
            onDoubleClick={() => onPlay(track, index)}
            onClick={coarse ? play : undefined}
          >
            <button
              type="button"
              className="music-row-cover"
              aria-label={`${active && playing ? t("Pause") : t("Play")} ${track.title}`}
              onClick={(e) => {
                e.stopPropagation();
                play();
              }}
            >
              <TrackCover track={track} />
              <span className="music-row-play">
                {active && playing ? <Pause size={16} /> : <Play size={16} />}
              </span>
            </button>
            <div className="music-row-main">
              <span className="music-row-title">{track.title}</span>
              <span className="music-row-artist">
                {track.artist || t("Unknown artist")}
                <span className="music-row-album-inline">
                  {track.album ? ` · ${track.album}` : ""}
                </span>
              </span>
            </div>
            <span className="music-row-album">{track.album}</span>
            <span className="music-row-time">
              {formatClock(track.durationMs / 1000)}
            </span>
            <button
              type="button"
              className={`music-heart${track.favorite ? " on" : ""}`}
              aria-label={
                track.favorite
                  ? t("Remove from favorites")
                  : t("Add to favorites")
              }
              aria-pressed={track.favorite}
              onClick={(e) => {
                e.stopPropagation();
                toggleFavorite(track);
              }}
            >
              <Heart
                size={15}
                fill={track.favorite ? "currentColor" : "none"}
              />
            </button>
            <span onClick={(e) => e.stopPropagation()}>
              <MoreMenu
                title={track.title}
                label={`${t("More")}: ${track.title}`}
                items={items(track, {
                  play: () => onPlay(track, index),
                  extra: extra?.(track, index),
                })}
              />
            </span>
          </div>
        );
      })}
      {footer}
    </div>
  );
}
