import { Music } from "lucide-react";
import { coverUrl, type Track } from "./api";

/** 歌的封面缩略图。没有封面时显示一个音符。 */
export function TrackCover({
  track,
  size = 96,
  className = "",
}: {
  track: Pick<Track, "id" | "hasCover" | "updatedAt">;
  size?: 96 | 256 | 640;
  className?: string;
}) {
  return (
    <CoverImage
      trackId={track.hasCover ? track.id : 0}
      version={track.updatedAt}
      size={size}
      className={className}
    />
  );
}

/** 按歌曲编号显示封面。编号是 0 表示没有封面。专辑、歌手、播放列表用它。 */
export function CoverImage({
  trackId,
  size = 96,
  version,
  className = "",
}: {
  trackId: number;
  size?: 96 | 256 | 640;
  version?: string;
  className?: string;
}) {
  return (
    <span className={`music-cover ${className}`} aria-hidden="true">
      {trackId > 0 ? (
        <img
          src={coverUrl(trackId, size, version)}
          alt=""
          loading="lazy"
          draggable={false}
        />
      ) : (
        <Music size={Math.max(14, Math.round(size / 5))} />
      )}
    </span>
  );
}

/** 播放列表的封面：最多 4 首歌的封面拼起来，没有封面的歌用音符。 */
export function MosaicCover({
  trackIds,
  className = "",
}: {
  trackIds: number[];
  className?: string;
}) {
  if (trackIds.length <= 1)
    return (
      <CoverImage trackId={trackIds[0] ?? 0} size={256} className={className} />
    );
  return (
    <span
      className={`music-cover music-mosaic ${className}`}
      aria-hidden="true"
    >
      {trackIds.slice(0, 4).map((id) => (
        <img
          key={id}
          src={coverUrl(id, 96)}
          alt=""
          loading="lazy"
          draggable={false}
        />
      ))}
    </span>
  );
}
