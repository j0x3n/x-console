import { FolderOpen, Plus, RefreshCw, X } from "lucide-react";
import { useState } from "react";
import Switch from "../../components/ui/Switch";
import { errorMessage, isNotLive } from "../../api/client";
import { NotLive, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  usePlaylists,
  useMusicSettings,
  usePutMusicSettings,
  useScanMusic,
} from "./api";
import FolderPicker from "./FolderPicker";
import "./i18n";
import "./music.css";

/** 设置里的“音乐”标签（B147）：选云盘里的音乐目录，重新扫描。 */
export default function MusicSettingsTab() {
  const t = useT();
  const settings = useMusicSettings();
  const save = usePutMusicSettings();
  const scan = useScanMusic();
  const playlists = usePlaylists();
  const [picking, setPicking] = useState(false);

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
          <div className="settings-inline">
            <button
              type="button"
              className="xc-btn small"
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
            <button
              type="button"
              className="xc-btn small"
              onClick={() => setPicking(true)}
            >
              <Plus size={14} /> {t("Add a folder")}
            </button>
          </div>
        </div>
        <p className="settings-note">
          {t(
            "Songs in these Drive folders, and the folders inside them, are added to the library. New files are picked up a few seconds after they are uploaded.",
          )}{" "}
          {t("Files in the trash are never added.")}
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
      </section>
      <section className="xc-card music-settings">
        <div className="xc-card-head">
          <h3>{t("Focus")}</h3>
        </div>
        <p className="settings-note">
          {t(
            "Tie the music to the pomodoro timer. Only the page you are looking at plays. If the browser blocks playing by itself, the player asks you to tap once.",
          )}
        </p>
        <div className="settings-line">
          <strong>{t("Play when a focus session starts")}</strong>
          <Switch
            checked={settings.data.focus.autoPlay}
            label={t("Play when a focus session starts")}
            disabled={save.isPending}
            onChange={(autoPlay) => change({ focus: { autoPlay } })}
          />
        </div>
        <div className="settings-line">
          <label htmlFor="music-focus-list">
            <strong>{t("Focus playlist")}</strong>
          </label>
          <div className="settings-inline">
            <select
              id="music-focus-list"
              className="xc-select"
              value={settings.data.focus.playlistId}
              disabled={save.isPending}
              onChange={(e) =>
                change({ focus: { playlistId: Number(e.target.value) } })
              }
            >
              <option value={0}>{t("The current queue")}</option>
              {(playlists.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="settings-line">
          <div>
            <strong>{t("Only use the focus playlist during focus")}</strong>
            <small>{t("The queue you had comes back when focus ends.")}</small>
          </div>
          <Switch
            checked={settings.data.focus.onlyFocusList}
            label={t("Only use the focus playlist during focus")}
            disabled={
              save.isPending ||
              !settings.data.focus.autoPlay ||
              settings.data.focus.playlistId === 0
            }
            onChange={(onlyFocusList) => change({ focus: { onlyFocusList } })}
          />
        </div>
        <div className="settings-line">
          <strong>{t("Pause when focus ends or is stopped")}</strong>
          <Switch
            checked={settings.data.focus.autoPause}
            label={t("Pause when focus ends or is stopped")}
            disabled={save.isPending}
            onChange={(autoPause) => change({ focus: { autoPause } })}
          />
        </div>
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
