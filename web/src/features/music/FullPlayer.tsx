import { ChevronDown, Heart } from "lucide-react";
import { useEffect, useState } from "react";
import { useT } from "../../contexts/LanguageContext";
import { useUpdateTrack } from "./api";
import { TrackCover } from "./Cover";
import Lyrics from "./Lyrics";
import QueueList from "./QueueList";
import {
  Controls,
  ModeButton,
  SeekBar,
  TimeText,
  VolumeControl,
} from "./Player";
import { useMainLeft } from "./PlayerHost";
import { currentTrack, usePlayer } from "./player";

const RATES = [0.75, 1, 1.25, 1.5, 2];

type Tab = "now" | "lyrics" | "queue";

/**
 * 全屏播放页（B147）：左边是封面和控制，右边是歌词或队列。
 * 手机上分成“播放、歌词、队列”三个标签。
 */
export default function FullPlayer() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const rate = usePlayer((s) => s.rate);
  const setRate = usePlayer((s) => s.setRate);
  const setFullOpen = usePlayer((s) => s.setFullOpen);
  const patchTrack = usePlayer((s) => s.patchTrack);
  const update = useUpdateTrack();
  const [tab, setTab] = useState<Tab>(() =>
    window.matchMedia?.("(max-width: 720px)").matches ? "now" : "lyrics",
  );
  const left = useMainLeft();

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (document.querySelector(".modal-backdrop")) return;
      setFullOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [setFullOpen]);

  if (!track) return null;
  const tabs: Array<[Tab, string]> = [
    ["now", "Now playing"],
    ["lyrics", "Lyrics"],
    ["queue", "Queue"],
  ];
  return (
    <div
      className="music-full"
      style={{ left }}
      role="dialog"
      aria-label={t("Now playing")}
      data-tab={tab}
    >
      <div className="music-full-head">
        <button
          type="button"
          className="music-btn"
          aria-label={t("Close the player")}
          title={t("Close the player")}
          onClick={() => setFullOpen(false)}
        >
          <ChevronDown size={20} />
        </button>
        <div className="xc-tabs music-full-tabs">
          {tabs.map(([id, label]) => (
            <button
              key={id}
              type="button"
              className={`${tab === id ? "active" : ""}${id === "now" ? " music-tab-now" : ""}`}
              onClick={() => setTab(id)}
            >
              {t(label)}
            </button>
          ))}
        </div>
      </div>
      <div className="music-full-body">
        <section className="music-full-now">
          <TrackCover track={track} size={640} className="music-full-cover" />
          <div className="music-full-title">
            <h2>{track.title}</h2>
            <p>
              {track.artist || t("Unknown artist")}
              {track.album ? ` · ${track.album}` : ""}
            </p>
          </div>
          <SeekBar />
          <div className="music-full-time">
            <TimeText />
          </div>
          <div className="music-full-controls">
            <ModeButton />
            <Controls big />
            <button
              type="button"
              className={`music-btn${track.favorite ? " on" : ""}`}
              aria-pressed={track.favorite}
              aria-label={
                track.favorite
                  ? t("Remove from favorites")
                  : t("Add to favorites")
              }
              title={
                track.favorite
                  ? t("Remove from favorites")
                  : t("Add to favorites")
              }
              onClick={() =>
                update.mutate(
                  { id: track.id, patch: { favorite: !track.favorite } },
                  { onSuccess: patchTrack },
                )
              }
            >
              <Heart
                size={17}
                fill={track.favorite ? "currentColor" : "none"}
              />
            </button>
          </div>
          <div className="music-full-extra">
            <label className="music-rate">
              <span>{t("Speed")}</span>
              <select
                className="xc-select"
                value={rate}
                onChange={(e) => setRate(Number(e.target.value))}
                aria-label={t("Speed")}
              >
                {RATES.map((r) => (
                  <option key={r} value={r}>
                    {r}x
                  </option>
                ))}
              </select>
            </label>
            <VolumeControl />
          </div>
        </section>
        <section className="music-full-side">
          {tab === "queue" ? <QueueList /> : <Lyrics trackId={track.id} />}
        </section>
      </div>
    </div>
  );
}
