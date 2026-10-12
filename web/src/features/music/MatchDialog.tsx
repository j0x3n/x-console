import { Link } from "react-router";
import { useCallback, useState } from "react";
import { Activity, Wand2 } from "lucide-react";
import { useServerEvent, type ServerEvent } from "../../api/events";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCheckSources,
  useMatchAll,
  useMusicSettings,
  usePending,
  usePutMusicSettings,
  type MatchProgress,
  type SourceCheck,
} from "./api";
import { sourceName } from "./PendingView";
import "./i18n";
import "./music.css";

const ORDER = ["lrclib", "netease", "qqmusic", "itunes", "musicbrainz"];
function bySourceOrder(a: string, b: string): number {
  return ORDER.indexOf(a) - ORDER.indexOf(b);
}

/**
 * 在线匹配（2026-10-11 从设置挪到音乐页）。默认自动匹配歌词和封面并写进文件，
 * 匹配得不对的歌可以在歌的菜单里手动看别的来源再选。这里是开关和批量操作。
 */
export default function MatchDialog({ onClose }: { onClose: () => void }) {
  const t = useT();
  const settings = useMusicSettings();
  const save = usePutMusicSettings();
  const matchAll = useMatchAll();
  const pending = usePending();
  const check = useCheckSources();
  const checked = new Map<string, SourceCheck>(
    (check.data ?? []).map((c) => [c.source, c]),
  );
  const [progress, setProgress] = useState<MatchProgress | null>(null);
  const onProgress = useCallback((e: ServerEvent) => {
    setProgress(e.data as MatchProgress);
  }, []);
  useServerEvent("music.match_progress", onProgress);

  const change = (patch: Parameters<typeof save.mutate>[0]) =>
    save.mutate(patch, {
      onSuccess: () => toast(t("Saved")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  const runMatch = (retry: boolean) =>
    matchAll.mutate(retry, {
      onSuccess: () => toast(t("Matching in the background")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });

  return (
    <Dialog open onClose={onClose} title={t("Online match")}>
      {settings.isPending ? (
        <Loading />
      ) : settings.isError ? (
        <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
      ) : (
        <div className="music-settings">
          <p className="music-muted">
            {t(
              "Songs without lyrics or a cover are looked up online and the result is written into the file. Only songs that clearly match (same title, a shared artist, length within 2 seconds) are used. The others wait in To confirm. If a match is wrong, choose lyrics or a cover from other sources in the song menu.",
            )}
          </p>
          <div className="music-option">
            <span>{t("Match new songs automatically")}</span>
            <Switch
              checked={settings.data.autoMatch}
              label={t("Match new songs automatically")}
              disabled={save.isPending}
              onChange={(autoMatch) => change({ autoMatch })}
            />
          </div>
          <div className="music-option">
            <span>
              {t("Write lyrics and covers into the song files")}
              <small className="music-muted">
                {t(
                  "This changes the files in Drive directly. The old version is not kept.",
                )}
              </small>
            </span>
            <Switch
              checked={settings.data.writeBack}
              label={t("Write lyrics and covers into the song files")}
              disabled={save.isPending}
              onChange={(writeBack) => change({ writeBack })}
            />
          </div>
          <h4 className="music-subhead">{t("Sources")}</h4>
          {Object.keys(settings.data.providers)
            .sort(bySourceOrder)
            .map((name) => (
              <div className="music-option" key={name}>
                <span>
                  {sourceName(name)}
                  {(name === "netease" || name === "qqmusic") && (
                    <small className="music-muted">
                      {t(
                        "No official interface. It may stop working at any time.",
                      )}
                    </small>
                  )}
                  <SourceStatus result={checked.get(name)} />
                </span>
                <Switch
                  checked={settings.data.providers[name]}
                  label={sourceName(name)}
                  disabled={save.isPending}
                  onChange={(on) => change({ providers: { [name]: on } })}
                />
              </div>
            ))}
          <div className="music-settings-actions">
            <button
              type="button"
              className="xc-btn primary"
              disabled={matchAll.isPending || progress?.running === true}
              onClick={() => runMatch(false)}
            >
              <Wand2 size={14} /> {t("Match now")}
            </button>
            <button
              type="button"
              className="xc-btn"
              disabled={matchAll.isPending || progress?.running === true}
              onClick={() => runMatch(true)}
            >
              {t("Try failed songs again")}
            </button>
            <button
              type="button"
              className="xc-btn"
              disabled={check.isPending}
              title={t(
                "Search every source once and show whether it works from the server",
              )}
              onClick={() =>
                check.mutate(undefined, {
                  onError: (e) =>
                    toast({ message: errorMessage(e), tone: "error" }),
                })
              }
            >
              <Activity size={14} />{" "}
              {check.isPending ? t("Checking…") : t("Check sources")}
            </button>
          </div>
          {progress?.running && (
            <p className="music-muted">
              {t("Matching")} {progress.done}/{progress.total}
              {progress.current ? ` · ${progress.current}` : ""}
            </p>
          )}
          {(pending.data?.length ?? 0) > 0 && (
            <p>
              <Link to="/music?view=pending" onClick={onClose}>
                {pending.data?.length}{" "}
                {t("songs are waiting for you to confirm")}
              </Link>
            </p>
          )}
        </div>
      )}
    </Dialog>
  );
}

/** 来源检查的结果（B169）：能不能用，搜到几条，失败的原因。 */
function SourceStatus({ result }: { result?: SourceCheck }) {
  const t = useT();
  if (!result) return null;
  if (!result.ok)
    return (
      <small className="music-source-check fail">
        {result.error || t("Failed")}
      </small>
    );
  return (
    <small className="music-source-check ok">
      {t("Works")} · {t("Lyrics")} {result.lyricsHits} · {t("Cover")}{" "}
      {result.coverHits} · {result.ms} ms
    </small>
  );
}
