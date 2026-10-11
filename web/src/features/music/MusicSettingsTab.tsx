import { FolderOpen, Plus, RefreshCw, Wand2, X } from "lucide-react";
import { useCallback, useState } from "react";
import { Link } from "react-router";
import { useServerEvent, type ServerEvent } from "../../api/events";
import Switch from "../../components/ui/Switch";
import { errorMessage, isNotLive } from "../../api/client";
import { NotLive, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useMatchAll,
  useMusicSettings,
  usePending,
  usePutMusicSettings,
  useScanMusic,
  type MatchProgress,
} from "./api";
import { sourceName } from "./PendingView";
import FolderPicker from "./FolderPicker";
import "./i18n";
import "./music.css";

/** 设置里的“音乐”标签（B147）：选云盘里的音乐目录，重新扫描。 */
export default function MusicSettingsTab() {
  const t = useT();
  const settings = useMusicSettings();
  const save = usePutMusicSettings();
  const scan = useScanMusic();
  const matchAll = useMatchAll();
  const pending = usePending();
  const [picking, setPicking] = useState(false);
  const [progress, setProgress] = useState<MatchProgress | null>(null);
  const onProgress = useCallback((e: ServerEvent) => {
    setProgress(e.data as MatchProgress);
  }, []);
  useServerEvent("music.match_progress", onProgress);

  if (settings.isPending) return <Loading />;
  if (settings.isError && isNotLive(settings.error))
    return <NotLive name={t("Music")} />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );

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
  const folders = settings.data.folders;
  const ids = folders.map((f) => f.id);
  const apply = (next: number[]) =>
    save.mutate(
      { folders: next },
      {
        onSuccess: () => toast(t("Saved")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );

  return (
    <div className="settings-grid">
      <section className="xc-card music-settings">
        <div className="xc-card-head">
          <h3>{t("Music folders")}</h3>
        </div>
        <p className="music-muted">
          {t(
            "Songs in these Drive folders, and the folders inside them, are added to the library. New files are picked up a few seconds after they are uploaded.",
          )}
        </p>
        <ul className="music-folder-list">
          {folders.map((f) => (
            <li key={f.id}>
              <FolderOpen size={14} />
              <span className="music-folder-path" title={f.path}>
                {f.path}
              </span>
              <button
                type="button"
                className="xc-btn small ghost"
                aria-label={`${t("Remove")} ${f.path}`}
                disabled={save.isPending}
                onClick={() => apply(ids.filter((id) => id !== f.id))}
              >
                <X size={13} />
              </button>
            </li>
          ))}
          {folders.length === 0 && (
            <li className="music-muted">{t("No music folder yet")}</li>
          )}
        </ul>
        <div className="music-settings-actions">
          <button
            type="button"
            className="xc-btn primary"
            onClick={() => setPicking(true)}
          >
            <Plus size={14} /> {t("Add a folder")}
          </button>
          <button
            type="button"
            className="xc-btn"
            disabled={scan.isPending || folders.length === 0}
            onClick={() =>
              scan.mutate(undefined, {
                onSuccess: () => toast(t("Scanning in the background")),
                onError: (e) =>
                  toast({ message: errorMessage(e), tone: "error" }),
              })
            }
          >
            <RefreshCw size={14} /> {t("Scan now")}
          </button>
        </div>
        <p className="music-muted">
          {t("Hidden folders and files in the trash are never added.")}
        </p>
      </section>
      <section className="xc-card music-settings">
        <div className="xc-card-head">
          <h3>{t("Online match")}</h3>
        </div>
        <p className="music-muted">
          {t(
            "Songs without lyrics or a cover are looked up online. Only songs that clearly match (same title, a shared artist, length within 2 seconds) are used. The others wait in To confirm.",
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
        </div>
        {progress?.running && (
          <p className="music-muted">
            {t("Matching")} {progress.done}/{progress.total}
            {progress.current ? ` · ${progress.current}` : ""}
          </p>
        )}
        {(pending.data?.length ?? 0) > 0 && (
          <p>
            <Link to="/music?view=pending">
              {pending.data?.length} {t("songs are waiting for you to confirm")}
            </Link>
          </p>
        )}
      </section>
      {picking && (
        <FolderPicker
          exclude={ids}
          onClose={() => setPicking(false)}
          onPick={(id) => {
            setPicking(false);
            apply([...ids, id]);
          }}
        />
      )}
    </div>
  );
}

const ORDER = ["lrclib", "netease", "qqmusic", "itunes", "musicbrainz"];
function bySourceOrder(a: string, b: string): number {
  return ORDER.indexOf(a) - ORDER.indexOf(b);
}
