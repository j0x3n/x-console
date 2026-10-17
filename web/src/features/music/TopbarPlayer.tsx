import {
  ChevronRight,
  ListMusic,
  Maximize2,
  Music,
  Pause,
  Pin,
  PinOff,
  Play,
  SkipForward,
  X,
} from "lucide-react";
import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { Link } from "react-router";
import { create } from "zustand";
import { useT } from "../../contexts/LanguageContext";
import { coverUrl } from "./api";
import { TrackCover } from "./Cover";
import Lyrics from "./LyricsView";
import QueueList from "./QueueList";
import { Controls, ModeButton, SeekBar, VolumeButton } from "./PlayerBar";
import { SleepButton } from "./SleepDialog";
import { formatClock } from "./lyrics";
import { currentTrack, usePlayer, useProgress } from "./player";
import "./i18n";
import "./music.css";

/*
 * 页头右上角的播放器（B168）。
 * 默认只有一个播放按钮，点了展开封面、暂停、下一首。
 * 点封面弹出迷你播放器：封面、完整控制、歌词，底部可以展开播放列表。
 * 迷你播放器固定后，换到哪个页面都还在。
 */

const STORE_KEY = "xc.music.mini";

interface Saved {
  expanded: boolean;
  pinned: boolean;
  queueOpen: boolean;
}

function load(): Saved {
  const fallback = { expanded: false, pinned: false, queueOpen: false };
  try {
    const raw = JSON.parse(localStorage.getItem(STORE_KEY) ?? "{}") as Record<
      string,
      unknown
    >;
    return {
      expanded: raw.expanded === true,
      pinned: raw.pinned === true,
      queueOpen: raw.queueOpen === true,
    };
  } catch {
    return fallback;
  }
}

function save(s: Saved) {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(s));
  } catch {
    // 存不了就只在这次打开期间有效
  }
}

interface MiniState extends Saved {
  /** 迷你播放器是否打开。固定时一直打开。 */
  open: boolean;
  setExpanded: (expanded: boolean) => void;
  toggleOpen: () => void;
  close: () => void;
  /** 点别处时收起没固定的迷你播放器。 */
  dismiss: () => void;
  togglePin: () => void;
  toggleQueue: () => void;
}

export const useTopbarPlayer = create<MiniState>()((set, get) => {
  const initial = load();
  const persist = (patch: Partial<Saved>) => {
    const { expanded, pinned, queueOpen } = { ...get(), ...patch };
    save({ expanded, pinned, queueOpen });
    set(patch);
  };
  return {
    ...initial,
    open: initial.pinned,
    setExpanded: (expanded) => persist({ expanded }),
    toggleOpen: () => {
      const { open, pinned } = get();
      // 已固定的再点一次封面，取消固定并收起
      if (open && pinned) persist({ pinned: false });
      set({ open: !open });
    },
    close: () => {
      if (get().pinned) persist({ pinned: false });
      set({ open: false });
    },
    dismiss: () => {
      if (!get().pinned) set({ open: false });
    },
    togglePin: () => persist({ pinned: !get().pinned }),
    toggleQueue: () => persist({ queueOpen: !get().queueOpen }),
  };
});

function MiniTime() {
  const time = useProgress((s) => s.time);
  const duration = useProgress((s) => s.duration);
  return (
    <div className="music-mini-time">
      <span>{formatClock(time)}</span>
      <span>{formatClock(duration)}</span>
    </div>
  );
}

function MiniPlayer() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const setFullOpen = usePlayer((s) => s.setFullOpen);
  const pinned = useTopbarPlayer((s) => s.pinned);
  const queueOpen = useTopbarPlayer((s) => s.queueOpen);
  const togglePin = useTopbarPlayer((s) => s.togglePin);
  const toggleQueue = useTopbarPlayer((s) => s.toggleQueue);
  const close = useTopbarPlayer((s) => s.close);
  const dismiss = useTopbarPlayer((s) => s.dismiss);
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    if (pinned) return;
    const onDown = (event: PointerEvent) => {
      const target = event.target as HTMLElement;
      if (ref.current?.contains(target)) return;
      // 点页头上的播放器按钮由按钮自己处理
      if (target.closest?.(".music-top")) return;
      // 弹窗和菜单（队列里的更多菜单、确认框）不算点在外面
      if (target.closest?.(".modal-backdrop, .xc-more-menu, .xc-more-backdrop"))
        return;
      dismiss();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (document.querySelector(".modal-backdrop")) return;
      dismiss();
    };
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [pinned, dismiss]);

  if (!track) return null;
  const pinLabel = pinned ? t("Unpin from every page") : t("Pin to every page");
  return createPortal(
    <section
      className="music-mini"
      role="dialog"
      aria-label={t("Now playing")}
      ref={ref}
    >
      <div className="music-mini-head">
        {track.hasCover && (
          <img
            className="music-mini-backdrop"
            src={coverUrl(track.id, 96, track.updatedAt)}
            alt=""
            aria-hidden="true"
            draggable={false}
          />
        )}
        <div className="music-mini-track">
          <TrackCover track={track} size={256} className="music-mini-cover" />
          <span className="music-row-main">
            <span className="music-row-title" title={track.title}>
              {track.title}
            </span>
            <span className="music-row-artist">
              {track.artist || t("Unknown artist")}
            </span>
          </span>
          <span className="music-mini-tools">
            <button
              type="button"
              className={`music-btn small${queueOpen ? " on" : ""}`}
              aria-label={t("Queue")}
              title={t("Queue")}
              aria-expanded={queueOpen}
              onClick={toggleQueue}
            >
              <ListMusic size={14} />
            </button>
            <button
              type="button"
              className={`music-btn small${pinned ? " on" : ""}`}
              aria-pressed={pinned}
              aria-label={pinLabel}
              title={pinLabel}
              onClick={togglePin}
            >
              {pinned ? <PinOff size={14} /> : <Pin size={14} />}
            </button>
            <button
              type="button"
              className="music-btn small"
              aria-label={t("Open the player")}
              title={t("Open the player")}
              onClick={() => setFullOpen(true)}
            >
              <Maximize2 size={14} />
            </button>
            <button
              type="button"
              className="music-btn small"
              aria-label={t("Close")}
              title={t("Close")}
              onClick={close}
            >
              <X size={14} />
            </button>
          </span>
        </div>
        <SeekBar />
        <MiniTime />
        {/* B191：左中右三段，播放按钮在正中间 */}
        <div className="music-mini-controls">
          <span className="music-mini-side">
            <ModeButton />
            <SleepButton />
          </span>
          <Controls />
          <span className="music-mini-side end">
            <VolumeButton horizontal />
          </span>
        </div>
      </div>
      <div className="music-mini-lyrics">
        <Lyrics trackId={track.id} />
      </div>
      <div className={`music-mini-queue${queueOpen ? " open" : ""}`}>
        <div className="music-mini-queue-inner">
          {queueOpen && <QueueList />}
        </div>
      </div>
    </section>,
    document.body,
  );
}

/** 收起时播放中的状态：三根跳动的竖条。 */
function PlayingBars() {
  return (
    <span className="music-bars" aria-hidden="true">
      <i />
      <i />
      <i />
    </span>
  );
}

export default function TopbarPlayer() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const playing = usePlayer((s) => s.playing);
  const blocked = usePlayer((s) => s.blocked);
  const toggle = usePlayer((s) => s.toggle);
  const resume = usePlayer((s) => s.resume);
  const next = usePlayer((s) => s.next);
  const expanded = useTopbarPlayer((s) => s.expanded);
  const setExpanded = useTopbarPlayer((s) => s.setExpanded);
  const open = useTopbarPlayer((s) => s.open);
  const toggleOpen = useTopbarPlayer((s) => s.toggleOpen);
  const dismiss = useTopbarPlayer((s) => s.dismiss);

  // 没有歌的时候只留一个去音乐页的入口
  if (!track)
    return (
      <Link
        to="/music"
        className="icon-button music-top-link"
        aria-label={t("Music")}
        title={t("Music")}
      >
        <Music size={16} />
      </Link>
    );

  const isPlaying = playing && !blocked;
  const play = blocked ? resume : toggle;
  // 暂停时一律折叠成一个播放按钮，接着播放时恢复原来的展开状态
  const shown = expanded && isPlaying;

  return (
    <div
      className={`music-top${shown ? " expanded" : ""}`}
      role="group"
      aria-label={t("Music player")}
    >
      {!shown && (
        <button
          type="button"
          className={`icon-button music-top-main${isPlaying ? " music-top-on" : ""}${blocked ? " music-top-blocked" : ""}`}
          aria-label={isPlaying ? t("Show the player") : t("Play")}
          title={
            blocked
              ? t("Tap to start playing")
              : isPlaying
                ? `${track.title} · ${track.artist || t("Unknown artist")}`
                : t("Play")
          }
          onClick={() => {
            if (!isPlaying) play();
            setExpanded(true);
          }}
        >
          {isPlaying ? <PlayingBars /> : <Play size={16} />}
        </button>
      )}
      <div className="music-top-extra" aria-hidden={!shown} inert={!shown}>
        <button
          type="button"
          className={`music-top-cover${open ? " on" : ""}`}
          aria-label={`${t("Now playing")}: ${track.title}`}
          title={`${track.title} · ${track.artist || t("Unknown artist")}`}
          aria-expanded={open}
          onClick={toggleOpen}
        >
          <TrackCover track={track} />
        </button>
        <button
          type="button"
          className={`icon-button${blocked ? " music-top-blocked" : ""}`}
          aria-label={isPlaying ? t("Pause") : t("Play")}
          title={
            blocked
              ? t("Tap to start playing")
              : isPlaying
                ? t("Pause")
                : t("Play")
          }
          onClick={play}
        >
          {isPlaying ? <Pause size={16} /> : <Play size={16} />}
        </button>
        <button
          type="button"
          className="icon-button"
          aria-label={t("Next song")}
          title={t("Next song")}
          onClick={() => next("manual")}
        >
          <SkipForward size={16} />
        </button>
        <button
          type="button"
          className="icon-button music-top-fold"
          aria-label={t("Collapse the player")}
          title={t("Collapse the player")}
          onClick={() => {
            setExpanded(false);
            dismiss();
          }}
        >
          <ChevronRight size={14} />
        </button>
      </div>
      {open && <MiniPlayer />}
    </div>
  );
}
