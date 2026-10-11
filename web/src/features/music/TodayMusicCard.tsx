import { Pause, Play, SkipForward } from "lucide-react";
import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { TrackCover } from "./Cover";
import { currentTrack, usePlayer } from "./player";
import "./i18n";
import "./music.css";

/** 今日页的“正在播放”卡片（B150）：封面、歌名、播放暂停和下一首。没有歌在播放条里时今日页不显示它。 */
export default function TodayMusicCard() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const playing = usePlayer((s) => s.playing);
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  if (!track) return <p className="music-muted">{t("Nothing is playing")}</p>;
  return (
    <div className="music-today">
      <Link to="/music" className="music-today-info">
        <TrackCover track={track} />
        <span className="music-row-main">
          <span className="music-row-title">{track.title}</span>
          <span className="music-row-artist">
            {track.artist || t("Unknown artist")}
          </span>
        </span>
      </Link>
      <button
        type="button"
        className="music-btn main"
        aria-label={playing ? t("Pause") : t("Play")}
        title={playing ? t("Pause") : t("Play")}
        onClick={toggle}
      >
        {playing ? <Pause size={17} /> : <Play size={17} />}
      </button>
      <button
        type="button"
        className="music-btn"
        aria-label={t("Next song")}
        title={t("Next song")}
        onClick={() => next("manual")}
      >
        <SkipForward size={17} />
      </button>
    </div>
  );
}
