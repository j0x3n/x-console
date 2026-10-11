import { create } from "zustand";
import {
  appendUnique,
  insertAfter,
  moveItem,
  nextMode,
  nextStep,
  prevStep,
  removeFrom,
  PLAY_MODES,
  type PlayMode,
} from "./queue";
import type { Track } from "./api";

/*
 * 全局播放器的状态（B147）。队列、当前歌、播放模式、音量放在这里，
 * 真正出声的 <audio> 在 PlayerHost 里跟着它走。播放进度变化很快，单独放在 useProgress，
 * 免得整个界面跟着每秒几次的进度重新渲染。
 */

const KEY = "xc.music.player";
const MAX_SAVED = 300;

export interface PlayerState {
  queue: Track[];
  currentId: number | null;
  playing: boolean;
  /** 浏览器拒绝了自动播放，要用户点一下。 */
  blocked: boolean;
  mode: PlayMode;
  /** 随机模式的种子，绕一圈后换一个。 */
  seed: number;
  volume: number;
  rate: number;
  fullOpen: boolean;

  /** 用 tracks 换掉队列，从 startId（默认第一首）开始播。 */
  play: (tracks: Track[], startId?: number) => void;
  /** 插在当前歌后面。队列是空的就直接开始播。 */
  playNext: (tracks: Track[]) => void;
  enqueue: (tracks: Track[]) => void;
  jumpTo: (id: number) => void;
  removeFromQueue: (id: number) => void;
  moveInQueue: (from: number, to: number) => void;
  clearQueue: () => void;
  next: (reason: "ended" | "manual") => void;
  prev: () => void;
  toggle: () => void;
  pause: () => void;
  resume: () => void;
  setMode: (mode: PlayMode) => void;
  cycleMode: () => void;
  setVolume: (volume: number) => void;
  setRate: (rate: number) => void;
  setFullOpen: (open: boolean) => void;
  setBlocked: (blocked: boolean) => void;
  /** 换回之前的队列，停在暂停状态（专注结束时用）。 */
  restore: (queue: Track[], currentId: number | null) => void;
  /** 歌曲信息改了（收藏、改名）时，同步队列里那一份。 */
  patchTrack: (track: Track) => void;
}

interface Saved {
  queue: Track[];
  currentId: number | null;
  mode: PlayMode;
  seed: number;
  volume: number;
  rate: number;
  time: number;
}

function validNumber(v: unknown, min: number, max: number, fallback: number) {
  return typeof v === "number" && Number.isFinite(v) && v >= min && v <= max
    ? v
    : fallback;
}

/** 读上次的状态。读不到或格式不对就当没有，页面照常用。 */
export function loadSaved(): Saved | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as Partial<Saved>;
    const queue = Array.isArray(s.queue)
      ? s.queue.filter(
          (t): t is Track =>
            !!t && typeof t.id === "number" && typeof t.title === "string",
        )
      : [];
    const currentId =
      typeof s.currentId === "number" && queue.some((t) => t.id === s.currentId)
        ? s.currentId
        : null;
    return {
      queue,
      currentId,
      mode: PLAY_MODES.includes(s.mode as PlayMode)
        ? (s.mode as PlayMode)
        : "sequence",
      seed: validNumber(s.seed, 0, 2 ** 32, 1),
      volume: validNumber(s.volume, 0, 1, 0.8),
      rate: validNumber(s.rate, 0.5, 3, 1),
      time: validNumber(s.time, 0, 24 * 3600, 0),
    };
  } catch {
    return null;
  }
}

const saved = loadSaved();
/** 刷新页面后要回到的播放位置（秒），播放器读到歌的元数据后用一次。 */
export const resume = { time: saved?.time ?? 0 };

let timer: ReturnType<typeof setTimeout> | undefined;

/** 把状态写进 localStorage。队列最多记 300 首，超出的丢掉后面的。 */
export function persist(time?: number) {
  const s = usePlayer.getState();
  const keep = s.queue.slice(0, MAX_SAVED);
  const data: Saved = {
    queue: keep,
    currentId: keep.some((t) => t.id === s.currentId) ? s.currentId : null,
    mode: s.mode,
    seed: s.seed,
    volume: s.volume,
    rate: s.rate,
    time: time ?? useProgress.getState().time,
  };
  try {
    localStorage.setItem(KEY, JSON.stringify(data));
  } catch {
    /* 记不住就算了 */
  }
}

function persistSoon() {
  clearTimeout(timer);
  timer = setTimeout(() => persist(), 400);
}

const initial = saved ?? {
  queue: [] as Track[],
  currentId: null,
  mode: "sequence" as PlayMode,
  seed: 1,
  volume: 0.8,
  rate: 1,
  time: 0,
};

const ids = (queue: Track[]) => queue.map((t) => t.id);

export const usePlayer = create<PlayerState>()((set, get) => ({
  queue: initial.queue,
  currentId: initial.currentId,
  playing: false,
  blocked: false,
  mode: initial.mode,
  seed: initial.seed,
  volume: initial.volume,
  rate: initial.rate,
  fullOpen: false,

  play: (tracks, startId) => {
    if (tracks.length === 0) return;
    const start =
      startId != null && tracks.some((t) => t.id === startId)
        ? startId
        : tracks[0].id;
    resume.time = 0;
    set({
      queue: tracks,
      currentId: start,
      playing: true,
      blocked: false,
      seed: Math.floor(Math.random() * 2 ** 32),
    });
  },
  playNext: (tracks) => {
    const s = get();
    if (tracks.length === 0) return;
    if (s.currentId == null || s.queue.length === 0) {
      get().play(tracks);
      return;
    }
    set({ queue: insertAfter(s.queue, s.currentId, tracks) });
  },
  enqueue: (tracks) => {
    const s = get();
    if (tracks.length === 0) return;
    if (s.currentId == null || s.queue.length === 0) {
      get().play(tracks);
      return;
    }
    set({ queue: appendUnique(s.queue, tracks) });
  },
  jumpTo: (id) => {
    if (!get().queue.some((t) => t.id === id)) return;
    resume.time = 0;
    set({ currentId: id, playing: true, blocked: false });
  },
  removeFromQueue: (id) => {
    const s = get();
    const r = removeFrom(s.queue, id, s.currentId, s.mode, s.seed);
    resume.time = 0;
    set({
      queue: r.list,
      currentId: r.current,
      playing: r.current == null ? false : s.playing,
    });
  },
  moveInQueue: (from, to) => set({ queue: moveItem(get().queue, from, to) }),
  clearQueue: () => set({ queue: [], currentId: null, playing: false }),
  next: (reason) => {
    const s = get();
    const step = nextStep(ids(s.queue), s.currentId, s.mode, s.seed, reason);
    if (step.id == null) {
      set({ playing: false });
      return;
    }
    resume.time = 0;
    set({
      currentId: step.id,
      playing: true,
      seed: step.wrapped ? Math.floor(Math.random() * 2 ** 32) : s.seed,
    });
  },
  prev: () => {
    const s = get();
    const id = prevStep(ids(s.queue), s.currentId, s.mode, s.seed);
    if (id == null) return;
    resume.time = 0;
    set({ currentId: id, playing: true });
  },
  toggle: () => {
    const s = get();
    if (s.currentId == null) return;
    set({ playing: !s.playing, blocked: false });
  },
  pause: () => set({ playing: false }),
  resume: () => {
    if (get().currentId != null) set({ playing: true, blocked: false });
  },
  setMode: (mode) => set({ mode }),
  cycleMode: () => set({ mode: nextMode(get().mode) }),
  setVolume: (volume) => set({ volume: Math.min(1, Math.max(0, volume)) }),
  setRate: (rate) => set({ rate }),
  setFullOpen: (fullOpen) => set({ fullOpen }),
  setBlocked: (blocked) =>
    set({ blocked, ...(blocked ? { playing: false } : {}) }),
  restore: (queue, currentId) => {
    resume.time = 0;
    set({
      queue,
      currentId: queue.some((t) => t.id === currentId) ? currentId : null,
      playing: false,
    });
  },
  patchTrack: (track) => {
    const s = get();
    if (!s.queue.some((t) => t.id === track.id)) return;
    set({ queue: s.queue.map((t) => (t.id === track.id ? track : t)) });
  },
}));

usePlayer.subscribe((s, prev) => {
  if (
    s.queue !== prev.queue ||
    s.currentId !== prev.currentId ||
    s.mode !== prev.mode ||
    s.seed !== prev.seed ||
    s.volume !== prev.volume ||
    s.rate !== prev.rate
  )
    persistSoon();
});

interface ProgressState {
  /** 当前播放位置，秒。 */
  time: number;
  /** 当前歌的总长，秒。读到元数据前是 0。 */
  duration: number;
}

export const useProgress = create<ProgressState>()(() => ({
  time: resume.time,
  duration: 0,
}));

/** 当前正在播放的歌，队列里没有时是 null。 */
export function currentTrack(s: PlayerState): Track | null {
  return s.queue.find((t) => t.id === s.currentId) ?? null;
}
