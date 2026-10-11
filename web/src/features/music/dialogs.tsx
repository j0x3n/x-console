import { useState } from "react";
import { create } from "zustand";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { errorMessage } from "../../api/client";
import { toast } from "../../hooks/useToast";
import {
  useAddToPlaylist,
  useLyrics,
  usePutLyrics,
  useCreatePlaylist,
  usePlaylists,
  useUpdateTrack,
  type Track,
} from "./api";
import { usePlayer } from "./player";
import SleepDialog from "./SleepDialog";

/*
 * 全局的两个小弹窗（B147）：加入播放列表、编辑歌曲信息。
 * 状态放在 store 里，歌曲列表、播放队列、云盘页都能打开，弹窗只挂一份。
 */
interface DialogState {
  addTo: Track[] | null;
  edit: Track | null;
  lyrics: Track | null;
  sleep: boolean;
}

export const useMusicDialogs = create<DialogState>()(() => ({
  addTo: null,
  edit: null,
  lyrics: null,
  sleep: false,
}));

export const openAddToPlaylist = (tracks: Track[]) =>
  useMusicDialogs.setState({ addTo: tracks });
export const openEditTrack = (track: Track) =>
  useMusicDialogs.setState({ edit: track });
export const openSleepDialog = () => useMusicDialogs.setState({ sleep: true });
export const openEditLyrics = (track: Track) =>
  useMusicDialogs.setState({ lyrics: track });

export default function MusicDialogs() {
  const { addTo, edit, lyrics, sleep } = useMusicDialogs();
  return (
    <>
      {addTo && (
        <AddToPlaylistDialog
          tracks={addTo}
          onClose={() => useMusicDialogs.setState({ addTo: null })}
        />
      )}
      {sleep && (
        <SleepDialog
          onClose={() => useMusicDialogs.setState({ sleep: false })}
        />
      )}
      {lyrics && (
        <EditLyricsDialog
          track={lyrics}
          onClose={() => useMusicDialogs.setState({ lyrics: null })}
        />
      )}
      {edit && (
        <EditTrackDialog
          track={edit}
          onClose={() => useMusicDialogs.setState({ edit: null })}
        />
      )}
    </>
  );
}

function AddToPlaylistDialog({
  tracks,
  onClose,
}: {
  tracks: Track[];
  onClose: () => void;
}) {
  const t = useT();
  const lists = usePlaylists();
  const add = useAddToPlaylist();
  const create = useCreatePlaylist();
  const [name, setName] = useState("");
  const ids = tracks.map((x) => x.id);
  const busy = add.isPending || create.isPending;

  const done = (listName: string) => {
    toast(`${t("Added to")} ${listName}`);
    onClose();
  };
  const fail = (e: unknown) =>
    toast({ message: errorMessage(e), tone: "error" });

  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Add to playlist")}
      description={
        tracks.length === 1 ? tracks[0].title : `${tracks.length} ${t("songs")}`
      }
    >
      <div className="music-pick-list">
        {(lists.data ?? []).map((p) => (
          <button
            key={p.id}
            type="button"
            className="music-pick-row"
            disabled={busy}
            onClick={() =>
              add.mutate(
                { id: p.id, trackIds: ids },
                { onSuccess: () => done(p.name), onError: fail },
              )
            }
          >
            <span>{p.name}</span>
            <small>
              {p.trackCount} {t("songs")}
            </small>
          </button>
        ))}
        {lists.data?.length === 0 && (
          <p className="music-muted">{t("No playlists yet")}</p>
        )}
      </div>
      <form
        className="music-pick-new"
        onSubmit={(e) => {
          e.preventDefault();
          const n = name.trim();
          if (!n) return;
          create.mutate(
            { name: n, trackIds: ids },
            { onSuccess: () => done(n), onError: fail },
          );
        }}
      >
        <input
          className="xc-input"
          value={name}
          maxLength={100}
          placeholder={t("New playlist name")}
          aria-label={t("New playlist name")}
          onChange={(e) => setName(e.target.value)}
        />
        <button className="xc-btn primary" disabled={busy || !name.trim()}>
          {t("Create and add")}
        </button>
      </form>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

function EditTrackDialog({
  track,
  onClose,
}: {
  track: Track;
  onClose: () => void;
}) {
  const t = useT();
  const update = useUpdateTrack();
  const [form, setForm] = useState({
    title: track.title,
    artist: track.artist,
    album: track.album,
    albumArtist: track.albumArtist,
  });
  const set = (key: keyof typeof form) => (value: string) =>
    setForm((f) => ({ ...f, [key]: value }));
  const fields: Array<[keyof typeof form, string]> = [
    ["title", "Title"],
    ["artist", "Artist"],
    ["album", "Album"],
    ["albumArtist", "Album artist"],
  ];
  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Edit song info")}
      description={t(
        "Saved in the library only. The file itself is not changed.",
      )}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!form.title.trim()) return;
          update.mutate(
            { id: track.id, patch: form },
            {
              onSuccess: (next) => {
                usePlayer.getState().patchTrack(next);
                toast(t("Saved"));
                onClose();
              },
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            },
          );
        }}
      >
        {fields.map(([key, label]) => (
          <div className="xc-field" key={key}>
            <label htmlFor={`music-edit-${key}`}>{t(label)}</label>
            <input
              id={`music-edit-${key}`}
              className="xc-input"
              value={form[key]}
              maxLength={300}
              onChange={(e) => set(key)(e.target.value)}
            />
          </div>
        ))}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={update.isPending || !form.title.trim()}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function EditLyricsDialog({
  track,
  onClose,
}: {
  track: Track;
  onClose: () => void;
}) {
  const t = useT();
  const current = useLyrics(track.id);
  const save = usePutLyrics();
  const [text, setText] = useState<string | null>(null);
  // 没改过时显示现有的歌词（带时间轴的还原成 LRC）。
  const shown =
    text ??
    (current.data?.lines ?? [])
      .map((l) => (l.timeMs == null ? l.text : `[${stamp(l.timeMs)}]${l.text}`))
      .join("\n");
  return (
    <Dialog
      open
      onClose={onClose}
      wide
      title={t("Edit lyrics")}
      description={`${track.title} · ${t("Plain text or LRC with time stamps like [01:23.45].")}`}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!shown.trim()) return;
          save.mutate(
            { id: track.id, text: shown },
            {
              onSuccess: () => {
                toast(t("Saved"));
                onClose();
              },
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            },
          );
        }}
      >
        <textarea
          className="xc-textarea music-lyrics-editor"
          rows={14}
          value={shown}
          aria-label={t("Lyrics")}
          onChange={(e) => setText(e.target.value)}
        />
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={save.isPending || !shown.trim()}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function stamp(ms: number): string {
  const total = Math.max(0, Math.round(ms / 10));
  const cs = total % 100;
  const sec = Math.floor(total / 100) % 60;
  const min = Math.floor(total / 6000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(min)}:${pad(sec)}.${pad(cs)}`;
}
