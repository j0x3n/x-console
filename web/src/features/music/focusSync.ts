import { onServerEvent } from "../../api/events";
import { queryClient } from "../../api/query";
import { unwrap } from "../../api/client";
import { musicApi, musicKeys, type MusicSettings, type Track } from "./api";
import { usePlayer } from "./player";

/*
 * 音乐和专注同步（B150）。番茄钟开始时按设置自动播放，结束或停止时暂停。
 * 播放在浏览器里，所以只有“当前看着的那个页面”会动，别的页面和设备不出声。
 * 浏览器要求用户点过页面才允许自动播放，被拒绝时播放条上显示“点一下开始播放”。
 */

export type FocusSettings = MusicSettings["focus"];

export type StartPlan =
  | { kind: "none" }
  | { kind: "resume" }
  | { kind: "playlist"; playlistId: number; swapQueue: boolean };

/** 专注开始时该做什么。 */
export function planFocusStart(
  s: FocusSettings,
  ctx: { visible: boolean; playing: boolean; hasCurrent: boolean },
): StartPlan {
  if (!s.autoPlay || !ctx.visible) return { kind: "none" };
  // 已经在听歌时不打断；要求专注期间只用专注列表时才换。
  if (s.playlistId > 0 && (s.onlyFocusList || !ctx.playing))
    return {
      kind: "playlist",
      playlistId: s.playlistId,
      swapQueue: s.onlyFocusList,
    };
  if (!ctx.playing && ctx.hasCurrent) return { kind: "resume" };
  return { kind: "none" };
}

export interface EndPlan {
  pause: boolean;
  /** 把专注开始时换掉的队列换回来。 */
  restoreQueue: boolean;
}

/**
 * 专注结束或停止时该做什么。换掉过队列时，暂停了或本来就没在放才换回来；
 * 还在放就让这首歌放完，不突然切回原来的队列。
 */
export function planFocusEnd(
  s: FocusSettings,
  ctx: { swapped: boolean; playing: boolean },
): EndPlan {
  const pause = s.autoPause && ctx.playing;
  return {
    pause,
    restoreQueue: ctx.swapped && (s.autoPause || !ctx.playing),
  };
}

interface Snapshot {
  queue: Track[];
  currentId: number | null;
}
let swapped: Snapshot | null = null;

async function loadSettings(): Promise<FocusSettings | null> {
  try {
    const cached = queryClient.getQueryData<MusicSettings>(musicKeys.settings);
    const data: MusicSettings | undefined =
      cached ??
      (await queryClient.fetchQuery({
        queryKey: musicKeys.settings,
        queryFn: () => unwrap(musicApi.GET("/music/settings")),
        staleTime: 30_000,
      }));
    return data?.focus ?? null;
  } catch {
    return null; // 接口还没上线或读不到，当作没开
  }
}

async function onStarted() {
  const s = await loadSettings();
  if (!s) return;
  const player = usePlayer.getState();
  const plan = planFocusStart(s, {
    visible: document.visibilityState === "visible" && document.hasFocus(),
    playing: player.playing,
    hasCurrent: player.currentId != null,
  });
  if (plan.kind === "resume") player.resume();
  if (plan.kind !== "playlist") return;
  try {
    const list = await unwrap(
      musicApi.GET("/music/playlists/{playlistId}", {
        params: { path: { playlistId: plan.playlistId } },
      }),
    );
    if (list.tracks.length === 0) return;
    const now = usePlayer.getState();
    if (plan.swapQueue && !swapped)
      swapped = { queue: now.queue, currentId: now.currentId };
    now.play(list.tracks);
  } catch {
    /* 读不到播放列表就不放 */
  }
}

async function onEnded() {
  const s = await loadSettings();
  if (!s) return;
  const player = usePlayer.getState();
  const plan = planFocusEnd(s, {
    swapped: swapped != null,
    playing: player.playing,
  });
  if (plan.pause) player.pause();
  if (plan.restoreQueue && swapped) {
    player.restore(swapped.queue, swapped.currentId);
    swapped = null;
  }
}

/** 订阅番茄钟的开始和结束事件。在全局播放器里调用一次。 */
export function startFocusSync(): () => void {
  return onServerEvent((event) => {
    if (event.topic === "focus.started") void onStarted();
    else if (event.topic === "focus.stopped" || event.topic === "focus.done")
      void onEnded();
  });
}

/** 测试用：清掉记下的队列。 */
export function resetFocusSync() {
  swapped = null;
}
