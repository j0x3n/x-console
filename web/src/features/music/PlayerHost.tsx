import { useEffect, useRef, useState } from "react";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { coverUrl, reportPlayed, streamUrl } from "./api";
import {
  currentTrack,
  persist,
  resume,
  usePlayer,
  useProgress,
} from "./player";

/*
 * 全局播放器的引擎（B147）。<audio> 不放进页面，放在模块里创建一次，
 * 切换页面时不会被拆掉，也就不会中断。这个组件把播放器状态同步给它，
 * 并处理：进度、读完一首后换下一首、系统媒体控制（锁屏、耳机键）、
 * 听满 30 秒或一半时上报一次、空格键。
 */

const audio: HTMLAudioElement | null =
  typeof Audio !== "undefined" ? new Audio() : null;
if (audio) audio.preload = "metadata";

/** 跳到某个位置（秒）。 */
export function seek(seconds: number) {
  if (!audio) return;
  const max = Number.isFinite(audio.duration) ? audio.duration : seconds;
  const t = Math.min(Math.max(0, seconds), max);
  audio.currentTime = t;
  useProgress.setState({ time: t });
}

const REPORT_SECONDS = 30;

interface Played {
  id: number | null;
  seconds: number;
  last: number;
  reported: boolean;
}

/** 页面主体左边缘的位置。桌面上播放条从左栏右边开始，手机上从 0 开始。 */
export function useMainLeft(): number {
  const [left, setLeft] = useState(0);
  useEffect(() => {
    const main = document.getElementById("main");
    if (!main) return;
    const update = () =>
      setLeft(Math.round(main.getBoundingClientRect().left) || 0);
    update();
    const observer =
      typeof ResizeObserver !== "undefined" ? new ResizeObserver(update) : null;
    observer?.observe(main);
    window.addEventListener("resize", update);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", update);
    };
  }, []);
  return left;
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.isContentEditable ||
    /^(INPUT|TEXTAREA|SELECT|BUTTON|A|SUMMARY|AUDIO|VIDEO)$/.test(
      target.tagName,
    ) ||
    target.closest("[role=dialog], [role=menu]") !== null
  );
}

export default function PlayerHost() {
  const t = useT();
  const track = usePlayer(currentTrack);
  const trackId = track?.id ?? null;
  const playing = usePlayer((s) => s.playing);
  const volume = usePlayer((s) => s.volume);
  const rate = usePlayer((s) => s.rate);
  const fullOpen = usePlayer((s) => s.fullOpen);
  const failures = useRef(0);
  const played = useRef<Played>({
    id: null,
    seconds: 0,
    last: 0,
    reported: false,
  });
  const changing = useRef(false);

  // 事件只绑一次，需要的最新状态从 store 里现读。
  useEffect(() => {
    if (!audio) return;
    let lastPosition = 0;
    const onTime = () => {
      const time = audio.currentTime;
      const duration = Number.isFinite(audio.duration) ? audio.duration : 0;
      useProgress.setState({ time, duration });
      const p = played.current;
      const delta = time - p.last;
      p.last = time;
      if (delta > 0 && delta < 2) p.seconds += delta;
      if (
        !p.reported &&
        p.id != null &&
        (p.seconds >= REPORT_SECONDS ||
          (duration > 0 && p.seconds >= duration / 2))
      ) {
        p.reported = true;
        reportPlayed(p.id, p.seconds);
      }
      if ("mediaSession" in navigator && time - lastPosition >= 1) {
        lastPosition = time;
        try {
          if (duration > 0)
            navigator.mediaSession.setPositionState({
              duration,
              position: Math.min(time, duration),
              playbackRate: audio.playbackRate || 1,
            });
        } catch {
          /* 有的浏览器不支持，不影响播放 */
        }
      }
    };
    const onMeta = () => {
      const duration = Number.isFinite(audio.duration) ? audio.duration : 0;
      if (resume.time > 0 && duration > 0) {
        audio.currentTime = Math.min(resume.time, Math.max(0, duration - 1));
      }
      resume.time = 0;
      useProgress.setState({ duration, time: audio.currentTime });
    };
    const onEnded = () => {
      const s = usePlayer.getState();
      const before = s.currentId;
      s.next("ended");
      const after = usePlayer.getState();
      // 单曲循环：编号没变，要自己回到开头重播。
      if (after.playing && after.currentId === before) {
        played.current = {
          id: before,
          seconds: 0,
          last: 0,
          reported: false,
        };
        audio.currentTime = 0;
        void audio.play().catch(() => undefined);
      }
    };
    const onPlaying = () => {
      failures.current = 0;
      changing.current = false;
    };
    const onPause = () => {
      persist(audio.currentTime);
      // 耳机拔掉、系统暂停等不是我们发起的暂停，同步回状态。
      if (audio.ended || changing.current || audio.error) return;
      if (usePlayer.getState().playing) usePlayer.getState().pause();
    };
    const onError = () => {
      if (!audio.getAttribute("src")) return;
      const code = audio.error?.code;
      if (code === MediaError.MEDIA_ERR_ABORTED) return;
      const s = usePlayer.getState();
      failures.current += 1;
      if (failures.current >= Math.max(1, s.queue.length)) {
        failures.current = 0;
        s.pause();
        toast(t("Cannot play these songs. Check that the files still exist."));
        return;
      }
      toast(t("Cannot play this song, skipped"));
      s.next("manual");
    };
    audio.addEventListener("timeupdate", onTime);
    audio.addEventListener("loadedmetadata", onMeta);
    audio.addEventListener("ended", onEnded);
    audio.addEventListener("playing", onPlaying);
    audio.addEventListener("pause", onPause);
    audio.addEventListener("error", onError);
    const save = window.setInterval(() => {
      if (!audio.paused) persist(audio.currentTime);
    }, 5000);
    const onHide = () => persist(audio.currentTime);
    window.addEventListener("pagehide", onHide);
    return () => {
      audio.removeEventListener("timeupdate", onTime);
      audio.removeEventListener("loadedmetadata", onMeta);
      audio.removeEventListener("ended", onEnded);
      audio.removeEventListener("playing", onPlaying);
      audio.removeEventListener("pause", onPause);
      audio.removeEventListener("error", onError);
      window.clearInterval(save);
      window.removeEventListener("pagehide", onHide);
    };
  }, [t]);

  // 换歌。
  useEffect(() => {
    if (!audio) return;
    if (trackId == null) {
      audio.pause();
      audio.removeAttribute("src");
      audio.removeAttribute("data-track");
      audio.load();
      useProgress.setState({ time: 0, duration: 0 });
      return;
    }
    if (audio.dataset.track !== String(trackId)) {
      audio.dataset.track = String(trackId);
      changing.current = true;
      audio.src = streamUrl(trackId);
      useProgress.setState({ time: resume.time, duration: 0 });
      played.current = { id: trackId, seconds: 0, last: 0, reported: false };
    }
  }, [trackId]);

  // 播放和暂停。
  useEffect(() => {
    if (!audio || trackId == null) return;
    if (!playing) {
      audio.pause();
      return;
    }
    audio.play().catch((err: unknown) => {
      if (err instanceof DOMException && err.name === "NotAllowedError")
        usePlayer.getState().setBlocked(true);
    });
  }, [playing, trackId]);

  useEffect(() => {
    if (!audio) return;
    audio.volume = volume;
  }, [volume]);
  useEffect(() => {
    if (!audio) return;
    audio.defaultPlaybackRate = rate;
    audio.playbackRate = rate;
  }, [rate, trackId]);

  // 系统媒体控制：锁屏、通知栏、耳机键。
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const ms = navigator.mediaSession;
    if (!track) {
      ms.metadata = null;
      return;
    }
    ms.metadata = new MediaMetadata({
      title: track.title,
      artist: track.artist,
      album: track.album,
      artwork: track.hasCover
        ? ([96, 256, 640] as const).map((size) => ({
            src: new URL(
              coverUrl(track.id, size, track.updatedAt),
              window.location.origin,
            ).href,
            sizes: `${size}x${size}`,
            type: "image/jpeg",
          }))
        : [],
    });
  }, [track]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    navigator.mediaSession.playbackState = playing ? "playing" : "paused";
  }, [playing]);
  useEffect(() => {
    if (!("mediaSession" in navigator) || !audio) return;
    const ms = navigator.mediaSession;
    const handlers: Array<
      [MediaSessionAction, MediaSessionActionHandler | null]
    > = [
      ["play", () => usePlayer.getState().resume()],
      ["pause", () => usePlayer.getState().pause()],
      ["previoustrack", () => usePlayer.getState().prev()],
      ["nexttrack", () => usePlayer.getState().next("manual")],
      [
        "seekto",
        (d) => {
          if (d.seekTime != null) seek(d.seekTime);
        },
      ],
      ["seekbackward", () => seek(audio.currentTime - 10)],
      ["seekforward", () => seek(audio.currentTime + 10)],
    ];
    for (const [action, handler] of handlers) {
      try {
        ms.setActionHandler(action, handler);
      } catch {
        /* 这个浏览器没有这个动作 */
      }
    }
    return () => {
      for (const [action] of handlers) {
        try {
          ms.setActionHandler(action, null);
        } catch {
          /* 同上 */
        }
      }
    };
  }, []);

  // 空格键播放和暂停。输入框、按钮、弹窗里不处理。
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== " " || event.repeat) return;
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (isTyping(event.target)) return;
      if (usePlayer.getState().currentId == null) return;
      event.preventDefault();
      usePlayer.getState().toggle();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  // 有歌在播放条里时给页面底部让位，并让 AI 浮钮上移（样式在 music.css）。
  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle("has-mini-player", trackId != null);
    root.classList.toggle("music-full-open", trackId != null && fullOpen);
    return () => {
      root.classList.remove("has-mini-player", "music-full-open");
    };
  }, [trackId, fullOpen]);

  return null;
}
