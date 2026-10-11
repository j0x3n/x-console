import { Moon } from "lucide-react";
import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCancelSleep, usePutSleep, useSleep } from "./api";
import { openSleepDialog } from "./dialogs";
import { formatRemaining, remainingMs } from "./sleep";

const PRESETS = [15, 30, 45, 60];

/** 现在的定时：剩多久，或者还剩几首。没有定时返回 null。 */
export function useSleepLabel(): string | null {
  const t = useT();
  const sleep = useSleep();
  const [now, setNow] = useState(() => Date.now());
  const endsAt = sleep.data?.mode === "time" ? sleep.data.endsAt : undefined;
  useEffect(() => {
    if (!endsAt) return;
    setNow(Date.now());
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [endsAt]);
  const data = sleep.data;
  if (!data?.active) return null;
  if (data.mode === "time")
    return `${t("Stops in")} ${formatRemaining(remainingMs(data.endsAt, now))}`;
  const left = data.tracksLeft ?? 1;
  return left <= 1
    ? t("Stops after this song")
    : `${t("Stops after")} ${left} ${t("songs")}`;
}

/** 播放条和全屏页上的月亮按钮，有定时时高亮并显示剩余。 */
export function SleepButton({ showLabel = false }: { showLabel?: boolean }) {
  const t = useT();
  const label = useSleepLabel();
  return (
    <button
      type="button"
      className={`music-btn music-sleep-btn${label ? " on" : ""}`}
      aria-label={label ?? t("Sleep timer")}
      title={label ?? t("Sleep timer")}
      onClick={openSleepDialog}
    >
      <Moon size={17} />
      {showLabel && label && <span className="music-sleep-label">{label}</span>}
    </button>
  );
}

/** 定时暂停的弹窗：几分钟后、播完这首、播完几首，已经设了的可以延长或取消。 */
export default function SleepDialog({ onClose }: { onClose: () => void }) {
  const t = useT();
  const sleep = useSleep();
  const put = usePutSleep();
  const cancel = useCancelSleep();
  const label = useSleepLabel();
  const [minutes, setMinutes] = useState("20");
  const [count, setCount] = useState("3");
  const busy = put.isPending || cancel.isPending;

  const set = (body: {
    minutes?: number;
    tracks?: number;
    extendMinutes?: number;
  }) =>
    put.mutate(body, {
      onSuccess: () => onClose(),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  const minutesN = Number(minutes);
  const countN = Number(count);
  const minutesOk =
    Number.isInteger(minutesN) && minutesN >= 1 && minutesN <= 720;
  const countOk = Number.isInteger(countN) && countN >= 2 && countN <= 20;

  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Sleep timer")}
      description={t("Pauses the music on every open page.")}
    >
      {sleep.data?.active && (
        <div className="music-sleep-now">
          <span>{label}</span>
          <span className="music-sleep-now-actions">
            {sleep.data.mode === "time" && (
              <button
                type="button"
                className="xc-btn small"
                disabled={busy}
                onClick={() => set({ extendMinutes: 15 })}
              >
                {t("Add 15 minutes")}
              </button>
            )}
            <button
              type="button"
              className="xc-btn small danger"
              disabled={busy}
              onClick={() =>
                cancel.mutate(undefined, {
                  onSuccess: onClose,
                  onError: (e) =>
                    toast({ message: errorMessage(e), tone: "error" }),
                })
              }
            >
              {t("Cancel timer")}
            </button>
          </span>
        </div>
      )}
      <div className="music-sleep-grid">
        {PRESETS.map((m) => (
          <button
            key={m}
            type="button"
            className="xc-btn"
            disabled={busy}
            onClick={() => set({ minutes: m })}
          >
            {m} {t("minutes")}
          </button>
        ))}
        <button
          type="button"
          className="xc-btn"
          disabled={busy}
          onClick={() => set({ tracks: 1 })}
        >
          {t("After this song")}
        </button>
      </div>
      <div className="music-sleep-custom">
        <label>
          <input
            className="xc-input"
            type="number"
            min={1}
            max={720}
            value={minutes}
            aria-label={t("Minutes")}
            onChange={(e) => setMinutes(e.target.value)}
          />
          <span>{t("minutes")}</span>
        </label>
        <button
          type="button"
          className="xc-btn primary"
          disabled={busy || !minutesOk}
          onClick={() => set({ minutes: minutesN })}
        >
          {t("Start timer")}
        </button>
      </div>
      <div className="music-sleep-custom">
        <label>
          <span>{t("After")}</span>
          <input
            className="xc-input"
            type="number"
            min={2}
            max={20}
            value={count}
            aria-label={t("Songs")}
            onChange={(e) => setCount(e.target.value)}
          />
          <span>{t("songs")}</span>
        </label>
        <button
          type="button"
          className="xc-btn primary"
          disabled={busy || !countOk}
          onClick={() => set({ tracks: countN })}
        >
          {t("Start timer")}
        </button>
      </div>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}
