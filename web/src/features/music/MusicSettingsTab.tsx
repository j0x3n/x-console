import { FolderOpen, Plus, RefreshCw, X } from "lucide-react";
import { useState } from "react";
import { errorMessage, isNotLive } from "../../api/client";
import { NotLive, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useMusicSettings, usePutMusicSettings, useScanMusic } from "./api";
import FolderPicker from "./FolderPicker";
import "./i18n";
import "./music.css";

/** 设置里的“音乐”标签（B147）：选云盘里的音乐目录，重新扫描。 */
export default function MusicSettingsTab() {
  const t = useT();
  const settings = useMusicSettings();
  const save = usePutMusicSettings();
  const scan = useScanMusic();
  const [picking, setPicking] = useState(false);

  if (settings.isPending) return <Loading />;
  if (settings.isError && isNotLive(settings.error))
    return <NotLive name={t("Music")} />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );

  const folders = settings.data.folders;
  const ids = folders.map((f) => f.id);
  const apply = (next: number[]) =>
    save.mutate(next, {
      onSuccess: () => toast(t("Saved")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });

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
