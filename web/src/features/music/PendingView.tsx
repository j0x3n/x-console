import { Captions, Image, ListChecks, SkipForward } from "lucide-react";
import { errorMessage } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useMatchTrack,
  usePending,
  useSkipMatch,
  type Candidate,
  type MatchPart,
  type Pending,
} from "./api";
import { TrackCover } from "./Cover";
import { formatClock } from "./lyrics";

/** 来源的显示名。 */
export function sourceName(source: string): string {
  switch (source) {
    case "lrclib":
      return "LRCLIB";
    case "netease":
      return "网易云音乐";
    case "qqmusic":
      return "QQ 音乐";
    case "itunes":
      return "iTunes";
    case "musicbrainz":
      return "MusicBrainz";
    default:
      return source;
  }
}

/**
 * 待确认（B148）：在线匹配拿不准的歌。每首歌列出几个候选，
 * 歌词和封面分开采用（B169），或者都不对就跳过（以后不再自动匹配）。
 */
export default function PendingView() {
  const t = useT();
  const list = usePending();
  if (list.isPending) return <Loading />;
  if (list.isError)
    return <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  if (list.data.length === 0)
    return (
      <EmptyState
        title={t("Nothing to confirm")}
        icon={<ListChecks size={28} />}
      >
        <span>
          {t(
            "Songs the online match is not sure about are listed here, with the closest candidates.",
          )}
        </span>
      </EmptyState>
    );
  return (
    <div className="music-pending">
      {list.data.map((item) => (
        <PendingCard key={item.track.id} item={item} />
      ))}
    </div>
  );
}

function PendingCard({ item }: { item: Pending }) {
  const t = useT();
  const match = useMatchTrack();
  const skip = useSkipMatch();
  const busy = match.isPending || skip.isPending;
  const track = item.track;
  // 歌词和封面分开采用（B169）：可以用一个来源的歌词、另一个来源的封面。
  const choose = (c: Candidate, part: MatchPart) =>
    match.mutate(
      {
        id: track.id,
        candidate: { source: c.source, sourceId: c.sourceId },
        parts: [part],
      },
      {
        onSuccess: () =>
          toast(part === "lyrics" ? t("Lyrics applied") : t("Cover applied")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  return (
    <section className="xc-card music-pending-card">
      <header className="music-pending-head">
        <TrackCover track={track} />
        <div className="music-row-main">
          <span className="music-row-title">{track.title}</span>
          <span className="music-row-artist">
            {track.artist || t("Unknown artist")}
            {track.album ? ` · ${track.album}` : ""} ·{" "}
            {formatClock(track.durationMs / 1000)}
          </span>
        </div>
        <button
          type="button"
          className="xc-btn small"
          disabled={busy}
          title={t("None of these is right. Do not match this song again.")}
          onClick={() =>
            skip.mutate(track.id, {
              onError: (e) =>
                toast({ message: errorMessage(e), tone: "error" }),
            })
          }
        >
          <SkipForward size={13} /> {t("None of these")}
        </button>
      </header>
      <ul className="music-candidates">
        {item.candidates.map((c) => (
          <li key={`${c.source}:${c.sourceId}`}>
            <div className="music-row-main">
              <span className="music-row-title">{c.title}</span>
              <span className="music-row-artist">
                {c.artist || t("Unknown artist")}
                {c.album ? ` · ${c.album}` : ""}
                {c.durationMs > 0
                  ? ` · ${formatClock(c.durationMs / 1000)}`
                  : ""}
              </span>
            </div>
            <span className="music-candidate-tags">
              <span className="xc-badge">{sourceName(c.source)}</span>
            </span>
            <span className="music-candidate-actions">
              {c.lyrics && (
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={busy}
                  title={t("Use the lyrics from this source")}
                  onClick={() => choose(c, "lyrics")}
                >
                  <Captions size={13} /> {t("Use lyrics")}
                </button>
              )}
              {c.cover && (
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={busy}
                  title={t("Use the cover from this source")}
                  onClick={() => choose(c, "cover")}
                >
                  <Image size={13} /> {t("Use cover")}
                </button>
              )}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
