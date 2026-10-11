import { KeyRound, Link2Off, Music } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import BrandMark from "../../../components/ui/BrandMark";
import { EmptyState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { musicApi, type PublicTrack } from "../api";
import { activeLine, formatClock } from "../lyrics";
import "../i18n";
import "../music.css";

/*
 * 播放列表分享页（B149）：https://<面板>/m/<token>，不用登录。
 * 只能播这个播放列表里的歌。有密码时先输密码，服务端发 1 小时的访问令牌，存在 sessionStorage。
 */

const KEY = (token: string) => `xc.music-share.${token}`;

function readAccess(token: string) {
  try {
    return sessionStorage.getItem(KEY(token)) ?? "";
  } catch {
    return "";
  }
}

function writeAccess(token: string, access: string) {
  try {
    sessionStorage.setItem(KEY(token), access);
  } catch {
    /* 存不了就只在这次页面里有效 */
  }
}

/** 公开接口的地址，有访问令牌时带上。 */
export function publicUrl(
  token: string,
  trackId: number,
  part: "stream" | "cover",
  access: string,
): string {
  const q = new URLSearchParams();
  if (access) q.set("access", access);
  if (part === "cover") q.set("size", "256");
  const qs = q.toString();
  return `/api/v1/public/music/${token}/tracks/${trackId}/${part}${qs ? `?${qs}` : ""}`;
}

export default function MusicSharePage({ token }: { token: string }) {
  const t = useT();
  const [access, setAccess] = useState(() => readAccess(token));
  const list = useQuery({
    queryKey: ["music-share", token, access],
    queryFn: () =>
      unwrap(
        musicApi.GET("/public/music/{token}", {
          params: { path: { token }, query: { access: access || undefined } },
        }),
      ),
    retry: false,
    meta: { silentError: true },
  });

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError) {
    const err = list.error;
    if (err instanceof ApiError && err.code === "share_code_required")
      body = (
        <CodeForm
          token={token}
          onUnlocked={(a) => {
            writeAccess(token, a);
            setAccess(a);
          }}
        />
      );
    else
      body = (
        <EmptyState
          title={t("This link does not exist or has ended")}
          icon={<Link2Off size={28} />}
        />
      );
  } else
    body = (
      <Playlist
        token={token}
        access={access}
        name={list.data.name}
        tracks={list.data.tracks}
      />
    );

  return (
    <div className="music-share">
      <header className="music-share-head">
        <BrandMark size={28} />
        <strong>{list.data?.name ?? t("Shared playlist")}</strong>
      </header>
      <main>{body}</main>
    </div>
  );
}

function CodeForm({
  token,
  onUnlocked,
}: {
  token: string;
  onUnlocked: (access: string) => void;
}) {
  const t = useT();
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await unwrap(
        musicApi.POST("/public/music/{token}/unlock", {
          params: { path: { token } },
          body: { code },
        }),
      );
      onUnlocked(r.access);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="xc-card music-share-code" onSubmit={submit}>
      <KeyRound size={22} />
      <strong>{t("This playlist has a password")}</strong>
      <input
        className="xc-input"
        type="password"
        autoFocus
        value={code}
        aria-label={t("Password")}
        onChange={(e) => setCode(e.target.value)}
      />
      {error && <small className="music-share-error">{error}</small>}
      <button className="xc-btn primary" disabled={busy || !code}>
        {t("Open")}
      </button>
    </form>
  );
}

function Playlist({
  token,
  access,
  name,
  tracks,
}: {
  token: string;
  access: string;
  name: string;
  tracks: PublicTrack[];
}) {
  const t = useT();
  const [index, setIndex] = useState(-1);
  const [time, setTime] = useState(0);
  const audio = useRef<HTMLAudioElement>(null);
  const current = index >= 0 ? tracks[index] : null;

  // 换歌就开始播。浏览器不让自动播放时用户点一下播放键。
  useEffect(() => {
    if (current && audio.current)
      void audio.current.play().catch(() => undefined);
  }, [current]);

  const lyrics = useQuery({
    queryKey: ["music-share-lyrics", token, current?.id, access],
    enabled: !!current?.hasLyrics,
    queryFn: () =>
      unwrap(
        musicApi.GET("/public/music/{token}/tracks/{trackId}/lyrics", {
          params: {
            path: { token, trackId: current?.id ?? 0 },
            query: { access: access || undefined },
          },
        }),
      ),
    retry: false,
    meta: { silentError: true },
  });
  const active = useMemo(
    () =>
      lyrics.data?.synced
        ? activeLine(lyrics.data.lines, Math.floor(time * 1000))
        : -1,
    [lyrics.data, time],
  );

  if (tracks.length === 0)
    return (
      <EmptyState
        title={t("This playlist is empty")}
        icon={<Music size={28} />}
      />
    );
  return (
    <>
      <p className="music-muted">
        {name} · {tracks.length} {t("songs")}
      </p>
      <div className="xc-card music-tracks">
        {tracks.map((track, i) => (
          <button
            key={track.id}
            type="button"
            className={`music-row music-share-row${i === index ? " active" : ""}`}
            onClick={() => setIndex(i)}
          >
            <span className="music-cover">
              {track.hasCover ? (
                <img
                  src={publicUrl(token, track.id, "cover", access)}
                  alt=""
                  loading="lazy"
                />
              ) : (
                <Music size={16} />
              )}
            </span>
            <span className="music-row-main">
              <span className="music-row-title">{track.title}</span>
              <span className="music-row-artist">
                {track.artist || t("Unknown artist")}
              </span>
            </span>
            <span className="music-row-time">
              {formatClock(track.durationMs / 1000)}
            </span>
          </button>
        ))}
      </div>
      {current && (
        <div className="music-share-player">
          <div className="music-share-now">
            <strong>{current.title}</strong>
            <span className="music-muted">{current.artist}</span>
            {lyrics.data && lyrics.data.lines.length > 0 && (
              <p className="music-share-lyric">
                {active >= 0 ? lyrics.data.lines[active].text : ""}
              </p>
            )}
          </div>
          <audio
            ref={audio}
            key={current.id}
            src={publicUrl(token, current.id, "stream", access)}
            controls
            autoPlay
            onTimeUpdate={(e) => setTime(e.currentTarget.currentTime)}
            onEnded={() =>
              setIndex((i) => (i + 1 < tracks.length ? i + 1 : -1))
            }
          />
        </div>
      )}
    </>
  );
}
