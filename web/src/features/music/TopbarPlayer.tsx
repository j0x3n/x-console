import {
  Captions,
  ListMusic,
  Maximize2,
  Music,
  Pause,
  Pin,
  PinOff,
  Play,
  X,
} from "lucide-react";
import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { Link } from "react-router";
import { create } from "zustand";
import { useT } from "../../contexts/LanguageContext";
import { TrackCover } from "./Cover";
import Lyrics from "./LyricsView";
import QueueList from "./QueueList";
import {
  Controls,
  ModeButton,
  SeekBar,
  TimeText,
  VolumeControl,
} from "./PlayerBar";
import { SleepButton } from "./SleepDialog";
import { currentTrack, usePlayer } from "./player";
import "./i18n";
import "./music.css";

/*
 * 页头右上角的迷你播放器：封面、播放暂停、队列、歌词四个按钮。
 * 点队列、歌词、封面在旁边弹出小组件，小组件上的图钉固定后，换到哪个页面都还在。
 */

type Panel = "now" | "queue" | "lyrics";
const PANELS: Panel[] = ["now", "queue", "lyrics"];
const PIN_KEY = "xc.music.pinned";

function loadPinned(): Panel[] {
  try {
    const raw = JSON.parse(localStorage.getItem(PIN_KEY) ?? "[]");
    return Array.isArray(raw)
      ? PANELS.filter((p) => (raw as unknown[]).includes(p))
      : [];
  } catch {
    return [];
  }
}

function savePinned(pinned: Panel[]) {
  try {
    localStorage.setItem(PIN_KEY, JSON.stringify(pinned));
  } catch {
    // 存不了就只在这次打开期间有效
  }
}

interface PopState {
  /** 点开但没固定的那个，点别处就收起。 */
  open: Panel | null;
  pinned: Panel[];
  toggle: (panel: Panel) => void;
  togglePin: (panel: Panel) => void;
  close: (panel: Panel) => void;
  closeOpen: () => void;
}

export const useTopbarPlayer = create<PopState>()((set, get) => ({
  open: null,
  pinned: loadPinned(),
  toggle: (panel) => {
    const { open, pinned } = get();
    if (pinned.includes(panel)) {
      // 已固定的再点一次，取消固定并收起
      const next = pinned.filter((p) => p !== panel);
      savePinned(next);
      set({ pinned: next, open: open === panel ? null : open });
    } else set({ open: open === panel ? null : panel });
  },
  togglePin: (panel) => {
    const { open, pinned } = get();
    if (pinned.includes(panel)) {
      const next = pinned.filter((p) => p !== panel);
      savePinned(next);
      set({ pinned: next, open: panel });
    } else {
      const next = PANELS.filter((p) => p === panel || pinned.includes(p));
      savePinned(next);
      set({ pinned: next, open: open === panel ? null : open });
    }
  },
  close: (panel) => {
    const { open, pinned } = get();
    const next = pinned.filter((p) => p !== panel);
    if (next.length !== pinned.length) savePinned(next);
    set({ pinned: next, open: open === panel ? null : open });
  },
  closeOpen: () => set({ open: null }),
}));

const PANEL_TITLE: Record<Panel, string> = {
  now: "Now playing",
  queue: "Queue",
  lyrics: "Lyrics",
};

function PopBody({ panel }: { panel: Panel }) {
  const t = useT();
  const track = usePlayer(currentTrack);
  const setFullOpen = usePlayer((s) => s.setFullOpen);
  const toggle = useTopbarPlayer((s) => s.toggle);
  if (!track) return null;
  if (panel === "queue") return <QueueList />;
  if (panel === "lyrics")
    return (
      <>
        <p className="music-pop-song">
          {track.title}
          <small>{track.artist || t("Unknown artist")}</small>
        </p>
        <div className="music-pop-lyrics">
          <Lyrics trackId={track.id} />
        </div>
      </>
    );
  return (
    <div className="music-pop-now">
      <div className="music-pop-track">
        <TrackCover track={track} size={256} className="music-pop-cover" />
        <span className="music-row-main">
          <span className="music-row-title">{track.title}</span>
          <span className="music-row-artist">
            {track.artist || t("Unknown artist")}
          </span>
        </span>
      </div>
      <SeekBar />
      <div className="music-pop-time">
        <TimeText />
      </div>
      <div className="music-pop-controls">
        <ModeButton />
        <Controls />
        <SleepButton />
      </div>
      <div className="music-pop-extra">
        <VolumeControl />
        <button
          type="button"
          className="music-btn music-pop-list"
          aria-label={t("Queue")}
          title={t("Queue")}
          onClick={() => toggle("queue")}
        >
          <ListMusic size={16} />
        </button>
        <button
          type="button"
          className="music-btn music-pop-list"
          aria-label={t("Lyrics")}
          title={t("Lyrics")}
          onClick={() => toggle("lyrics")}
        >
          <Captions size={16} />
        </button>
        <button
          type="button"
          className="music-btn"
          aria-label={t("Open the player")}
          title={t("Open the player")}
          onClick={() => setFullOpen(true)}
        >
          <Maximize2 size={16} />
        </button>
      </div>
    </div>
  );
}

function Pop({ panel, pinned }: { panel: Panel; pinned: boolean }) {
  const t = useT();
  const togglePin = useTopbarPlayer((s) => s.togglePin);
  const close = useTopbarPlayer((s) => s.close);
  return (
    <section
      className={`music-pop music-pop-${panel}`}
      role="dialog"
      aria-label={t(PANEL_TITLE[panel])}
    >
      <header className="music-pop-head">
        <strong>{t(PANEL_TITLE[panel])}</strong>
        <span className="music-pop-tools">
          <button
            type="button"
            className={`music-btn small${pinned ? " on" : ""}`}
            aria-pressed={pinned}
            aria-label={
              pinned ? t("Unpin from every page") : t("Pin to every page")
            }
            title={pinned ? t("Unpin from every page") : t("Pin to every page")}
            onClick={() => togglePin(panel)}
          >
            {pinned ? <PinOff size={14} /> : <Pin size={14} />}
          </button>
          <button
            type="button"
            className="music-btn small"
            aria-label={t("Close")}
            title={t("Close")}
            onClick={() => close(panel)}
          >
            <X size={14} />
          </button>
        </span>
      </header>
      <div className="music-pop-body">
        <PopBody panel={panel} />
      </div>
    </section>
  );
}

function PopStack() {
  const open = useTopbarPlayer((s) => s.open);
  const pinned = useTopbarPlayer((s) => s.pinned);
  const closeOpen = useTopbarPlayer((s) => s.closeOpen);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open == null) return;
    const onDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (ref.current?.contains(target)) return;
      // 点顶栏上的播放器按钮由按钮自己处理
      if ((target as HTMLElement).closest?.(".music-top")) return;
      // 弹窗和菜单（队列里的更多菜单、确认框）不算点在外面
      if (
        (target as HTMLElement).closest?.(
          ".modal-backdrop, .xc-more-menu, .xc-more-backdrop",
        )
      )
        return;
      closeOpen();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (document.querySelector(".modal-backdrop")) return;
      closeOpen();
    };
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, closeOpen]);

  const shown = PANELS.filter((p) => p === open || pinned.includes(p));
  if (shown.length === 0) return null;
  return createPortal(
    <div className="music-pop-stack" ref={ref}>
      {shown.map((p) => (
        <Pop key={p} panel={p} pinned={pinned.includes(p)} />
      ))}
    </div>,
    document.body,
  );
}

export default function TopbarPlayer() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const playing = usePlayer((s) => s.playing);
  const blocked = usePlayer((s) => s.blocked);
  const toggle = usePlayer((s) => s.toggle);
  const resume = usePlayer((s) => s.resume);
  const open = useTopbarPlayer((s) => s.open);
  const pinned = useTopbarPlayer((s) => s.pinned);
  const togglePanel = useTopbarPlayer((s) => s.toggle);

  // 没有歌的时候只留一个去音乐页的入口
  if (!track)
    return (
      <Link
        to="/music"
        className="icon-button"
        aria-label={t("Music")}
        title={t("Music")}
      >
        <Music size={16} />
      </Link>
    );

  const on = (p: Panel) => open === p || pinned.includes(p);
  return (
    <div className="music-top" role="group" aria-label={t("Music player")}>
      <button
        type="button"
        className="music-top-cover"
        aria-label={`${t("Now playing")}: ${track.title}`}
        title={`${track.title} · ${track.artist || t("Unknown artist")}`}
        aria-expanded={on("now")}
        onClick={() => togglePanel("now")}
      >
        <TrackCover track={track} />
      </button>
      <button
        type="button"
        className={`icon-button${blocked ? " music-top-blocked" : ""}`}
        aria-label={playing ? t("Pause") : t("Play")}
        title={
          blocked ? t("Tap to start playing") : playing ? t("Pause") : t("Play")
        }
        onClick={blocked ? resume : toggle}
      >
        {playing && !blocked ? <Pause size={16} /> : <Play size={16} />}
      </button>
      <button
        type="button"
        className={`icon-button music-top-list${on("queue") ? " music-top-on" : ""}`}
        aria-label={t("Queue")}
        title={t("Queue")}
        aria-expanded={on("queue")}
        onClick={() => togglePanel("queue")}
      >
        <ListMusic size={16} />
      </button>
      <button
        type="button"
        className={`icon-button music-top-lyrics${on("lyrics") ? " music-top-on" : ""}`}
        aria-label={t("Lyrics")}
        title={t("Lyrics")}
        aria-expanded={on("lyrics")}
        onClick={() => togglePanel("lyrics")}
      >
        <Captions size={16} />
      </button>
      <PopStack />
    </div>
  );
}
