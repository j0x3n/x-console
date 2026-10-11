import {
  Maximize2,
  MoveRight,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Shuffle,
  SkipBack,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
} from "lucide-react";
import { lazy, Suspense } from "react";
import { useT } from "../../contexts/LanguageContext";
import { TrackCover } from "./Cover";
import MusicDialogs from "./dialogs";
import { formatClock } from "./lyrics";
import { seek, useMainLeft } from "./PlayerHost";
import PlayerHost from "./PlayerHost";
import { modeLabel, type PlayMode } from "./queue";
import { currentTrack, usePlayer, useProgress } from "./player";
import "./i18n";
import "./music.css";

const FullPlayer = lazy(() => import("./FullPlayer"));

/** 播放模式的图标。 */
export function ModeIcon({
  mode,
  size = 17,
}: {
  mode: PlayMode;
  size?: number;
}) {
  const Icon = {
    sequence: MoveRight,
    loop: Repeat,
    one: Repeat1,
    shuffle: Shuffle,
  }[mode];
  return <Icon size={size} />;
}

/** 进度条。拖动时跳到那个位置。 */
export function SeekBar({ className = "" }: { className?: string }) {
  const t = useT();
  const time = useProgress((s) => s.time);
  const duration = useProgress((s) => s.duration);
  return (
    <input
      type="range"
      className={`music-seek ${className}`}
      min={0}
      max={Math.max(1, Math.floor(duration))}
      step={1}
      value={Math.min(Math.floor(time), Math.max(1, Math.floor(duration)))}
      disabled={duration <= 0}
      aria-label={t("Playback position")}
      style={{
        ["--music-progress" as string]: `${duration > 0 ? (time / duration) * 100 : 0}%`,
      }}
      onChange={(e) => seek(Number(e.target.value))}
    />
  );
}

export function TimeText() {
  const time = useProgress((s) => s.time);
  const duration = useProgress((s) => s.duration);
  return (
    <span className="music-time">
      {formatClock(time)} / {formatClock(duration)}
    </span>
  );
}

/** 上一首、播放暂停、下一首。 */
export function Controls({ big = false }: { big?: boolean }) {
  const t = useT();
  const playing = usePlayer((s) => s.playing);
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  const prev = usePlayer((s) => s.prev);
  const size = big ? 22 : 17;
  return (
    <div className={`music-controls${big ? " big" : ""}`}>
      <button
        type="button"
        className="music-btn"
        aria-label={t("Previous song")}
        title={t("Previous song")}
        onClick={prev}
      >
        <SkipBack size={size} />
      </button>
      <button
        type="button"
        className="music-btn main"
        aria-label={playing ? t("Pause") : t("Play")}
        title={playing ? t("Pause") : t("Play")}
        onClick={toggle}
      >
        {playing ? <Pause size={size + 2} /> : <Play size={size + 2} />}
      </button>
      <button
        type="button"
        className="music-btn"
        aria-label={t("Next song")}
        title={t("Next song")}
        onClick={() => next("manual")}
      >
        <SkipForward size={size} />
      </button>
    </div>
  );
}

export function VolumeControl() {
  const t = useT();
  const volume = usePlayer((s) => s.volume);
  const setVolume = usePlayer((s) => s.setVolume);
  const Icon = volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2;
  return (
    <div className="music-volume">
      <button
        type="button"
        className="music-btn"
        aria-label={volume === 0 ? t("Unmute") : t("Mute")}
        title={volume === 0 ? t("Unmute") : t("Mute")}
        onClick={() => setVolume(volume === 0 ? 0.8 : 0)}
      >
        <Icon size={17} />
      </button>
      <input
        type="range"
        className="music-seek music-volume-range"
        min={0}
        max={100}
        value={Math.round(volume * 100)}
        aria-label={t("Volume")}
        style={{ ["--music-progress" as string]: `${volume * 100}%` }}
        onChange={(e) => setVolume(Number(e.target.value) / 100)}
      />
    </div>
  );
}

export function ModeButton() {
  const t = useT();
  const mode = usePlayer((s) => s.mode);
  const cycle = usePlayer((s) => s.cycleMode);
  return (
    <button
      type="button"
      className={`music-btn${mode !== "sequence" ? " on" : ""}`}
      aria-label={t(modeLabel(mode))}
      title={t(modeLabel(mode))}
      onClick={cycle}
    >
      <ModeIcon mode={mode} />
    </button>
  );
}

function MiniBar() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const blocked = usePlayer((s) => s.blocked);
  const resume = usePlayer((s) => s.resume);
  const setFullOpen = usePlayer((s) => s.setFullOpen);
  const left = useMainLeft();
  if (!track) return null;
  return (
    <div
      className="music-mini"
      style={{ left }}
      role="region"
      aria-label={t("Music player")}
    >
      <SeekBar className="music-mini-seek" />
      <div className="music-mini-body">
        <button
          type="button"
          className="music-mini-info"
          onClick={() => setFullOpen(true)}
          aria-label={`${t("Open the player")}: ${track.title}`}
        >
          <TrackCover track={track} />
          <span className="music-row-main">
            <span className="music-row-title">{track.title}</span>
            <span className="music-row-artist">
              {blocked
                ? t("Tap to start playing")
                : track.artist || t("Unknown artist")}
            </span>
          </span>
        </button>
        <span className="music-mini-controls">
          <Controls />
        </span>
        <span className="music-mini-time">
          <TimeText />
        </span>
        <span className="music-mini-volume">
          <ModeButton />
          <VolumeControl />
        </span>
        <button
          type="button"
          className="music-btn music-mini-expand"
          aria-label={t("Open the player")}
          title={t("Open the player")}
          onClick={() => (blocked ? resume() : setFullOpen(true))}
        >
          <Maximize2 size={16} />
        </button>
      </div>
    </div>
  );
}

/** 全局音乐播放器：引擎、底部播放条、全屏页、两个小弹窗。挂在 GlobalPanels 里。 */
export default function MusicPlayer() {
  const fullOpen = usePlayer((s) => s.fullOpen && s.currentId != null);
  return (
    <>
      <PlayerHost />
      <MiniBar />
      {fullOpen && (
        <Suspense fallback={null}>
          <FullPlayer />
        </Suspense>
      )}
      <MusicDialogs />
    </>
  );
}
