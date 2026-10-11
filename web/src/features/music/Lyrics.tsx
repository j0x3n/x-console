import { useEffect, useMemo, useRef } from "react";
import { useT } from "../../contexts/LanguageContext";
import { Loading } from "../../components/ui/States";
import { useLyrics } from "./api";
import { activeLine } from "./lyrics";
import { seek } from "./PlayerHost";
import { useProgress } from "./player";

/**
 * 滚动歌词（B147）。有时间轴时当前行高亮并跟着滚，点一行跳到那一行；
 * 没有时间轴就是普通文字，自己滚动。
 */
export default function Lyrics({ trackId }: { trackId: number }) {
  const t = useT();
  const lyrics = useLyrics(trackId);
  const lines = useMemo(() => lyrics.data?.lines ?? [], [lyrics.data]);
  const synced = lyrics.data?.synced === true;
  // 只订阅“第几行”，进度每秒变几次，行没变时不重新渲染。
  const active = useProgress((s) =>
    synced ? activeLine(lines, Math.floor(s.time * 1000)) : -1,
  );
  const box = useRef<HTMLDivElement>(null);
  const current = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const el = current.current;
    const parent = box.current;
    if (!el || !parent) return;
    const target = el.offsetTop - parent.clientHeight / 2 + el.clientHeight / 2;
    parent.scrollTo({ top: Math.max(0, target), behavior: "smooth" });
  }, [active]);

  if (lyrics.isPending) return <Loading />;
  if (lyrics.isError || lines.length === 0)
    return <p className="music-muted music-lyrics-empty">{t("No lyrics")}</p>;

  return (
    <div className="music-lyrics" ref={box}>
      {lines.map((line, i) =>
        synced && line.timeMs != null ? (
          <button
            key={i}
            type="button"
            ref={i === active ? current : undefined}
            className={`music-lyric${i === active ? " active" : ""}`}
            onClick={() => seek((line.timeMs ?? 0) / 1000)}
          >
            {line.text || " "}
          </button>
        ) : (
          <p key={i} className="music-lyric plain">
            {line.text || " "}
          </p>
        ),
      )}
    </div>
  );
}
