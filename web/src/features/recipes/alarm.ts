import { useEffect, useState } from "react";
import type { CookTimer } from "./api";

/*
 * 计时器到点的提示音和倒数（B209）。
 * 提示音用 Web Audio 现场生成，不加音频文件。浏览器要求先有一次点击才能出声，
 * 所以开始计时的按钮里先调用 unlockAudio()。
 */

let ctx: AudioContext | null = null;

function audio(): AudioContext | null {
  try {
    if (!ctx) {
      const Ctor =
        window.AudioContext ??
        (window as unknown as { webkitAudioContext?: typeof AudioContext })
          .webkitAudioContext;
      if (!Ctor) return null;
      ctx = new Ctor();
    }
    return ctx;
  } catch {
    return null;
  }
}

/** 在用户点击里调用一次，之后到点时才能出声。 */
export function unlockAudio() {
  const a = audio();
  if (a && a.state === "suspended") void a.resume().catch(() => {});
}

/** 响三组“嘀嘀”，大约 2.5 秒。不能出声时什么都不做。 */
export function playAlarm() {
  const a = audio();
  if (!a) return;
  try {
    if (a.state === "suspended") void a.resume().catch(() => {});
    const start = a.currentTime + 0.05;
    for (let group = 0; group < 3; group++) {
      for (let beep = 0; beep < 2; beep++) {
        const at = start + group * 0.8 + beep * 0.22;
        const osc = a.createOscillator();
        const gain = a.createGain();
        osc.type = "sine";
        osc.frequency.value = 880;
        gain.gain.setValueAtTime(0.0001, at);
        gain.gain.exponentialRampToValueAtTime(0.4, at + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.16);
        osc.connect(gain).connect(a.destination);
        osc.start(at);
        osc.stop(at + 0.18);
      }
    }
  } catch {
    // 出不了声就算了，还有推送
  }
}

/** 现在还剩几秒。在走的按 endsAt 算，暂停的用返回时的剩余秒数。 */
export function remaining(timer: CookTimer, now: number): number {
  if (timer.state === "done") return 0;
  if (timer.state === "paused" || !timer.endsAt) return timer.remainingSeconds;
  return Math.max(0, Math.ceil((Date.parse(timer.endsAt) - now) / 1000));
}

/** 时间到了：服务端说 done，或者前端倒数到 0。 */
export function isDue(timer: CookTimer, now: number): boolean {
  return (
    timer.state === "done" ||
    (timer.state === "running" && remaining(timer, now) === 0)
  );
}

/** 有在走的计时器时每秒刷新一次的当前时间。 */
export function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [active]);
  return now;
}

/** 先到点的在前；暂停的按剩余时间排在后面。 */
export function sortTimers(timers: CookTimer[], now: number): CookTimer[] {
  const rank = (t: CookTimer) =>
    t.state === "done" ? 0 : t.state === "running" ? 1 : 2;
  return [...timers].sort(
    (a, b) =>
      rank(a) - rank(b) || remaining(a, now) - remaining(b, now) || a.id - b.id,
  );
}
