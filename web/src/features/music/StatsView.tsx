import { BarChart3 } from "lucide-react";
import { useState } from "react";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { Segmented } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { useStats } from "./api";

/** 听了多久，用 “1 小时 5 分”“12 分”“45 秒” 的写法。 */
export function listenText(t: (s: string) => string, seconds: number): string {
  if (seconds < 60) return `${seconds} ${t("seconds")}`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} ${t("minutes")}`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} ${t("hours")} ${m} ${t("minutes")}` : `${h} ${t("hours")}`;
}

const DAYS = [7, 30, 90] as const;

/**
 * 听歌统计（B150）：总时长、专注期间听的时长、听得最多的歌和歌手。
 * 只给数字，不下结论。一次播放指听满 30 秒或一半以上。
 */
export default function StatsView() {
  const t = useT();
  const [days, setDays] = useState<number>(30);
  const stats = useStats(days);
  if (stats.isPending) return <Loading />;
  if (stats.isError)
    return <ErrorState error={stats.error} onRetry={() => stats.refetch()} />;
  const s = stats.data;
  return (
    <>
      <div className="music-toolbar-row">
        <Segmented
          value={String(days)}
          onChange={(v) => setDays(Number(v))}
          label={t("Time range")}
          options={DAYS.map((d) => ({
            value: String(d),
            label: `${d} ${t("days")}`,
          }))}
        />
      </div>
      {s.plays === 0 ? (
        <EmptyState
          title={t("Nothing played in this period")}
          icon={<BarChart3 size={28} />}
        />
      ) : (
        <>
          <StatStrip label={t("Listening")}>
            <StatCard
              label={t("Listening time")}
              value={listenText(t, s.totalSeconds)}
            />
            <StatCard label={t("Plays")} value={s.plays} />
            <StatCard
              label={t("During focus")}
              value={listenText(t, s.focusSeconds)}
              foot={`${s.focusPlays} ${t("plays")}`}
            />
          </StatStrip>
          <div className="music-stats-grid">
            <Ranking
              title={t("Most played songs")}
              rows={s.topTracks.map((r) => ({
                key: `${r.trackId ?? 0}:${r.title}`,
                name: r.title,
                sub: r.artist,
                plays: r.plays,
                seconds: r.seconds,
              }))}
            />
            <Ranking
              title={t("Most played artists")}
              rows={s.topArtists.map((r) => ({
                key: r.artist,
                name: r.artist || t("Unknown artist"),
                plays: r.plays,
                seconds: r.seconds,
              }))}
            />
            <Ranking
              title={t("Most played during focus")}
              rows={s.focusTopTracks.map((r) => ({
                key: `${r.trackId ?? 0}:${r.title}`,
                name: r.title,
                sub: r.artist,
                plays: r.plays,
                seconds: r.seconds,
              }))}
            />
          </div>
        </>
      )}
    </>
  );
}

function Ranking({
  title,
  rows,
}: {
  title: string;
  rows: {
    key: string;
    name: string;
    sub?: string;
    plays: number;
    seconds: number;
  }[];
}) {
  const t = useT();
  return (
    <section className="xc-card music-ranking">
      <div className="xc-card-head">
        <h3>{title}</h3>
      </div>
      {rows.length === 0 ? (
        <p className="music-muted">{t("No data")}</p>
      ) : (
        <ol>
          {rows.map((r) => (
            <li key={r.key}>
              <span className="music-row-main">
                <span className="music-row-title">{r.name}</span>
                {r.sub && <span className="music-row-artist">{r.sub}</span>}
              </span>
              <span className="music-row-artist">
                {r.plays} {t("plays")} · {listenText(t, r.seconds)}
              </span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
