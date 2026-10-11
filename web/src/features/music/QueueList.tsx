import { ArrowDown, ArrowUp, X } from "lucide-react";
import { useState } from "react";
import MoreMenu from "../../components/ui/MoreMenu";
import { useT } from "../../contexts/LanguageContext";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { TrackCover } from "./Cover";
import { usePlayer } from "./player";
import { useTrackMenu } from "./trackActions";

/**
 * 播放队列（B147）。桌面上可以拖动一行排序，触屏用菜单里的上移、下移。
 * 随机模式下队列保持原来的顺序，实际播放顺序另算。
 */
export default function QueueList() {
  const t = useT();
  const queue = usePlayer((s) => s.queue);
  const currentId = usePlayer((s) => s.currentId);
  const jumpTo = usePlayer((s) => s.jumpTo);
  const move = usePlayer((s) => s.moveInQueue);
  const remove = usePlayer((s) => s.removeFromQueue);
  const clear = usePlayer((s) => s.clearQueue);
  const { items } = useTrackMenu();
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  const [over, setOver] = useState<number | null>(null);

  if (queue.length === 0)
    return (
      <p className="music-muted music-lyrics-empty">{t("Queue is empty")}</p>
    );

  return (
    <div className="music-queue">
      <div className="music-queue-head">
        <span>
          {queue.length} {t("songs")}
        </span>
        <button
          type="button"
          className="xc-btn small ghost"
          onClick={async () => {
            if (
              await confirmAction({
                title: t("Clear the queue?"),
                description: t("Playback stops. Your songs are not deleted."),
                confirmLabel: t("Clear"),
                danger: true,
              })
            )
              clear();
          }}
        >
          {t("Clear")}
        </button>
      </div>
      {queue.map((track, index) => (
        <div
          key={track.id}
          className={`music-queue-row${track.id === currentId ? " active" : ""}${over === index && dragFrom !== index ? " over" : ""}`}
          draggable
          onDragStart={() => setDragFrom(index)}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(index);
          }}
          onDragEnd={() => {
            setDragFrom(null);
            setOver(null);
          }}
          onDrop={(e) => {
            e.preventDefault();
            if (dragFrom != null) move(dragFrom, index);
            setDragFrom(null);
            setOver(null);
          }}
        >
          <button
            type="button"
            className="music-queue-main"
            onClick={() => jumpTo(track.id)}
          >
            <TrackCover track={track} />
            <span className="music-row-main">
              <span className="music-row-title">{track.title}</span>
              <span className="music-row-artist">
                {track.artist || t("Unknown artist")}
              </span>
            </span>
          </button>
          <MoreMenu
            title={track.title}
            label={`${t("More")}: ${track.title}`}
            items={items(track, {
              play: () => jumpTo(track.id),
              extra: [
                {
                  key: "up",
                  label: t("Move up"),
                  icon: <ArrowUp size={14} />,
                  onSelect: () => move(index, index - 1),
                },
                {
                  key: "down",
                  label: t("Move down"),
                  icon: <ArrowDown size={14} />,
                  onSelect: () => move(index, index + 1),
                },
                {
                  key: "remove",
                  label: t("Remove from queue"),
                  icon: <X size={14} />,
                  danger: true,
                  onSelect: () => remove(track.id),
                },
              ],
            })}
          />
        </div>
      ))}
    </div>
  );
}
