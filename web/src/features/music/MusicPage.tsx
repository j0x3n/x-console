import {
  ListMusic,
  Music,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Settings,
  Shuffle,
  Trash2,
  UserRound,
} from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useAlbums,
  useArtists,
  useCreatePlaylist,
  useDeletePlaylist,
  useMusicSettings,
  usePlaylist,
  usePlaylists,
  useRenamePlaylist,
  useScanMusic,
  useSetPlaylistItems,
  useTracks,
  type Track,
  type TrackFilter,
} from "./api";
import { CoverImage, MosaicCover } from "./Cover";
import { formatClock } from "./lyrics";
import { usePlayer } from "./player";
import TrackList from "./TrackList";
import "./i18n";
import "./music.css";

export type MusicView =
  | "songs"
  | "albums"
  | "artists"
  | "playlists"
  | "favorites"
  | "recent";
const VIEWS: MusicView[] = [
  "songs",
  "albums",
  "artists",
  "playlists",
  "favorites",
  "recent",
];

/** 地址里的视图名，认不出的当作“全部歌曲”。 */
export function parseView(value: string | null): MusicView {
  return VIEWS.includes(value as MusicView) ? (value as MusicView) : "songs";
}

const TITLES: Record<MusicView, string> = {
  songs: "All songs",
  albums: "Albums",
  artists: "Artists",
  playlists: "Playlists",
  favorites: "Favorites",
  recent: "Recently played",
};

/** 音乐：歌曲、专辑、歌手、播放列表。播放放在全局播放器里，这里只负责找歌和排歌。 */
export default function MusicPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState("");
  const settings = useMusicSettings();
  const scan = useScanMusic();
  const view = parseView(params.get("view"));
  const album = params.get("album");
  const albumArtist = params.get("albumArtist") ?? "";
  const artist = params.get("artist");
  const playlistId = Number(params.get("playlist")) || null;

  const go = (next: Record<string, string | null>) =>
    setParams(next as Record<string, string>, { replace: false });
  const hasFolders = (settings.data?.folders.length ?? 0) > 0;

  let body;
  if (settings.isPending) body = <Loading />;
  else if (settings.isError && isNotLive(settings.error))
    body = <NotLive name={t("Music")} icon={<Music size={28} />} />;
  else if (settings.isError)
    body = (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  else if (!hasFolders && view !== "playlists")
    body = (
      <EmptyState title={t("No music folder yet")} icon={<Music size={28} />}>
        <span>
          {t(
            "Choose the Drive folders that hold your songs. New files in them are added automatically.",
          )}
        </span>
        <Link className="xc-btn primary small" to="/settings/music">
          <Settings size={14} /> {t("Choose music folders")}
        </Link>
      </EmptyState>
    );
  else if (view === "albums" && album == null)
    body = (
      <AlbumGrid
        q={q}
        onOpen={(a, aa) => go({ view: "albums", album: a, albumArtist: aa })}
      />
    );
  else if (view === "albums")
    body = (
      <TrackSource
        title={album ?? ""}
        filter={{ album: album ?? "" }}
        albumArtist={albumArtist}
        q={q}
        back={() => go({ view: "albums" })}
      />
    );
  else if (view === "artists" && artist == null)
    body = (
      <ArtistGrid q={q} onOpen={(a) => go({ view: "artists", artist: a })} />
    );
  else if (view === "artists")
    body = (
      <TrackSource
        title={artist || t("Unknown artist")}
        filter={{ artist: artist ?? "" }}
        q={q}
        back={() => go({ view: "artists" })}
      />
    );
  else if (view === "playlists" && playlistId == null)
    body = (
      <PlaylistGrid
        q={q}
        onOpen={(id) => go({ view: "playlists", playlist: String(id) })}
      />
    );
  else if (view === "playlists")
    body = (
      <PlaylistView
        id={playlistId ?? 0}
        q={q}
        back={() => go({ view: "playlists" })}
      />
    );
  else
    body = (
      <TrackSource
        title={t(TITLES[view])}
        filter={
          view === "favorites"
            ? { favorite: true }
            : view === "recent"
              ? { sort: "recent" }
              : {}
        }
        q={q}
        root
      />
    );

  return (
    <div className="xc-page music-page">
      <PageHeading
        title={t(TITLES[view])}
        aside={
          <button
            className="xc-btn"
            title={t("Scan music folders")}
            disabled={scan.isPending || !hasFolders}
            onClick={() =>
              scan.mutate(undefined, {
                onSuccess: () => toast(t("Scanning in the background")),
                onError: (e) =>
                  toast({ message: errorMessage(e), tone: "error" }),
              })
            }
          >
            <RefreshCw size={14} /> {t("Scan")}
          </button>
        }
      />
      {view === "songs" && !album && hasFolders && <Stats />}
      {hasFolders || view === "playlists" ? (
        <Toolbar
          end={
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search songs, artists, albums and lyrics")}
              clearLabel={t("Clear")}
            />
          }
        />
      ) : null}
      {body}
    </div>
  );
}

function Stats() {
  const t = useT();
  const songs = useTracks({});
  const albums = useAlbums();
  const artists = useArtists();
  const lists = usePlaylists();
  return (
    <StatStrip label={t("Library")}>
      <StatCard label={t("Songs")} value={songs.total} />
      <StatCard label={t("Albums")} value={albums.data?.length ?? 0} />
      <StatCard label={t("Artists")} value={artists.data?.length ?? 0} />
      <StatCard label={t("Playlists")} value={lists.data?.length ?? 0} />
    </StatStrip>
  );
}

/** 播放全部和随机播放。队列用传进来的歌。 */
function PlayButtons({
  load,
  disabled,
}: {
  load: () => Promise<Track[]>;
  disabled?: boolean;
}) {
  const t = useT();
  const run = async (shuffle: boolean) => {
    try {
      const tracks = await load();
      if (tracks.length === 0) return;
      const s = usePlayer.getState();
      s.play(tracks);
      if (shuffle) s.setMode("shuffle");
      else if (s.mode === "shuffle") s.setMode("sequence");
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };
  return (
    <div className="music-head-actions">
      <button
        className="xc-btn primary"
        disabled={disabled}
        onClick={() => run(false)}
      >
        <Play size={14} /> {t("Play all")}
      </button>
      <button className="xc-btn" disabled={disabled} onClick={() => run(true)}>
        <Shuffle size={14} /> {t("Shuffle")}
      </button>
    </div>
  );
}

/** 一组歌：全部歌曲、我喜欢的、最近播放、专辑里的歌、歌手的歌。 */
function TrackSource({
  title,
  filter,
  q,
  root,
  back,
  albumArtist,
}: {
  title: string;
  filter: TrackFilter;
  q: string;
  root?: boolean;
  back?: () => void;
  albumArtist?: string;
}) {
  const t = useT();
  const list = useTracks({ ...filter, q });
  const items = useMemo(
    () =>
      albumArtist === undefined || albumArtist === ""
        ? list.items
        : list.items.filter((x) => (x.albumArtist || x.artist) === albumArtist),
    [list.items, albumArtist],
  );
  // 要播放全部时，先把剩下的页都拉完。
  const loadAll = async (): Promise<Track[]> => {
    let r = list;
    let guard = 0;
    while (r.hasNextPage && guard++ < 100) {
      const next = await r.fetchNextPage();
      if (!next.data) break;
      r = {
        ...r,
        items: next.data.pages.flatMap((p) => p.items),
        hasNextPage: next.hasNextPage,
      };
    }
    const all = r.items;
    return albumArtist
      ? all.filter((x) => (x.albumArtist || x.artist) === albumArtist)
      : all;
  };

  if (list.isPending) return <Loading />;
  if (list.isError)
    return <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  return (
    <>
      {!root && (
        <div className="music-detail-head">
          <button className="xc-btn small ghost" onClick={back}>
            ← {t("Back")}
          </button>
          <h2>{title}</h2>
          <span className="music-muted">
            {items.length} {t("songs")}
          </span>
        </div>
      )}
      <div className="music-toolbar-row">
        <PlayButtons load={loadAll} disabled={items.length === 0} />
        {list.isFetching && !list.isFetchingNextPage && (
          <span className="music-muted">{t("Loading…")}</span>
        )}
      </div>
      {items.length === 0 ? (
        <EmptyState
          title={q ? t("No song matches") : emptyTitle(filter, t)}
          icon={<Music size={28} />}
        />
      ) : (
        <TrackList
          tracks={items}
          onPlay={(track) => usePlayer.getState().play(items, track.id)}
          footer={
            list.hasNextPage ? (
              <div className="music-more">
                <button
                  className="xc-btn small"
                  disabled={list.isFetchingNextPage}
                  onClick={() => list.fetchNextPage()}
                >
                  {t("Load more")} ({items.length}/{list.total})
                </button>
              </div>
            ) : null
          }
        />
      )}
    </>
  );
}

function emptyTitle(filter: TrackFilter, t: (s: string) => string): string {
  if (filter.favorite) return t("No favorite songs yet");
  if (filter.sort === "recent") return t("Nothing played yet");
  return t("No songs yet");
}

function AlbumGrid({
  q,
  onOpen,
}: {
  q: string;
  onOpen: (album: string, albumArtist: string) => void;
}) {
  const t = useT();
  const albums = useAlbums();
  const needle = q.trim().toLowerCase();
  if (albums.isPending) return <Loading />;
  if (albums.isError)
    return <ErrorState error={albums.error} onRetry={() => albums.refetch()} />;
  const shown = albums.data.filter(
    (a) =>
      !needle || `${a.album} ${a.albumArtist}`.toLowerCase().includes(needle),
  );
  if (shown.length === 0)
    return (
      <EmptyState title={t("No album matches")} icon={<Music size={28} />} />
    );
  return (
    <div className="music-grid">
      {shown.map((a) => (
        <button
          key={`${a.album}\u0000${a.albumArtist}`}
          className="music-card"
          onClick={() => onOpen(a.album, a.albumArtist)}
        >
          <CoverImage
            trackId={a.coverTrackId}
            size={256}
            className="music-card-cover"
          />
          <strong>{a.album || t("Unknown album")}</strong>
          <small>
            {a.albumArtist || t("Unknown artist")}
            {a.year > 0 ? ` · ${a.year}` : ""} · {a.trackCount} {t("songs")}
          </small>
        </button>
      ))}
    </div>
  );
}

function ArtistGrid({
  q,
  onOpen,
}: {
  q: string;
  onOpen: (artist: string) => void;
}) {
  const t = useT();
  const artists = useArtists();
  const needle = q.trim().toLowerCase();
  if (artists.isPending) return <Loading />;
  if (artists.isError)
    return (
      <ErrorState error={artists.error} onRetry={() => artists.refetch()} />
    );
  const shown = artists.data.filter(
    (a) => !needle || a.artist.toLowerCase().includes(needle),
  );
  if (shown.length === 0)
    return (
      <EmptyState
        title={t("No artist matches")}
        icon={<UserRound size={28} />}
      />
    );
  return (
    <div className="music-grid">
      {shown.map((a) => (
        <button
          key={a.artist}
          className="music-card"
          onClick={() => onOpen(a.artist)}
        >
          <CoverImage
            trackId={a.coverTrackId}
            size={256}
            className="music-card-cover round"
          />
          <strong>{a.artist || t("Unknown artist")}</strong>
          <small>
            {a.albumCount} {t("albums")} · {a.trackCount} {t("songs")}
          </small>
        </button>
      ))}
    </div>
  );
}

function PlaylistGrid({
  q,
  onOpen,
}: {
  q: string;
  onOpen: (id: number) => void;
}) {
  const t = useT();
  const lists = usePlaylists();
  const create = useCreatePlaylist();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const needle = q.trim().toLowerCase();
  if (lists.isPending) return <Loading />;
  if (lists.isError)
    return <ErrorState error={lists.error} onRetry={() => lists.refetch()} />;
  const shown = lists.data.filter(
    (p) => !needle || p.name.toLowerCase().includes(needle),
  );
  return (
    <>
      <div className="music-toolbar-row">
        <button className="xc-btn primary" onClick={() => setCreating(true)}>
          <Plus size={14} /> {t("New playlist")}
        </button>
      </div>
      {shown.length === 0 ? (
        <EmptyState
          title={
            lists.data.length === 0
              ? t("No playlists yet")
              : t("No playlist matches")
          }
          icon={<ListMusic size={28} />}
        />
      ) : (
        <div className="music-grid">
          {shown.map((p) => (
            <button
              key={p.id}
              className="music-card"
              onClick={() => onOpen(p.id)}
            >
              <MosaicCover
                trackIds={p.coverTrackIds}
                className="music-card-cover"
              />
              <strong>{p.name}</strong>
              <small>
                {p.trackCount} {t("songs")} · {formatClock(p.durationMs / 1000)}
              </small>
            </button>
          ))}
        </div>
      )}
      <Dialog
        open={creating}
        onClose={() => setCreating(false)}
        title={t("New playlist")}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const n = name.trim();
            if (!n) return;
            create.mutate(
              { name: n },
              {
                onSuccess: (p) => {
                  setCreating(false);
                  setName("");
                  onOpen(p.id);
                },
                onError: (err) =>
                  toast({ message: errorMessage(err), tone: "error" }),
              },
            );
          }}
        >
          <div className="xc-field">
            <label htmlFor="music-new-playlist">{t("Name")}</label>
            <input
              id="music-new-playlist"
              className="xc-input"
              value={name}
              maxLength={100}
              autoFocus
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="xc-dialog-actions">
            <button
              type="button"
              className="xc-btn"
              onClick={() => setCreating(false)}
            >
              {t("Cancel")}
            </button>
            <button
              className="xc-btn primary"
              disabled={create.isPending || !name.trim()}
            >
              {t("Create")}
            </button>
          </div>
        </form>
      </Dialog>
    </>
  );
}

function PlaylistView({
  id,
  q,
  back,
}: {
  id: number;
  q: string;
  back: () => void;
}) {
  const t = useT();
  const navigate = useNavigate();
  const list = usePlaylist(id);
  const rename = useRenamePlaylist();
  const remove = useDeletePlaylist();
  const setItems = useSetPlaylistItems();
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState("");
  const needle = q.trim().toLowerCase();

  if (list.isPending) return <Loading />;
  if (list.isError)
    return <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  const data = list.data;
  const tracks = data.tracks;
  const shown = needle
    ? tracks.filter((x) =>
        `${x.title} ${x.artist} ${x.album}`.toLowerCase().includes(needle),
      )
    : tracks;
  const ids = tracks.map((x) => x.id);
  const save = (next: number[]) =>
    setItems.mutate(
      { id, trackIds: next },
      { onError: (e) => toast({ message: errorMessage(e), tone: "error" }) },
    );
  const swap = (index: number, to: number) => {
    if (to < 0 || to >= ids.length) return;
    const next = [...ids];
    [next[index], next[to]] = [next[to], next[index]];
    save(next);
  };
  return (
    <>
      <div className="music-detail-head">
        <button className="xc-btn small ghost" onClick={back}>
          ← {t("Back")}
        </button>
        <h2>{data.name}</h2>
        <span className="music-muted">
          {data.trackCount} {t("songs")} · {formatClock(data.durationMs / 1000)}
        </span>
        <span className="xc-spacer" />
        <button
          className="xc-btn small"
          onClick={() => {
            setName(data.name);
            setRenaming(true);
          }}
        >
          <Pencil size={13} /> {t("Rename")}
        </button>
        <button
          className="xc-btn small danger"
          onClick={async () => {
            if (
              await confirmAction({
                title: `${t("Delete playlist")} ${data.name}?`,
                description: t("The songs stay in your library."),
              })
            )
              remove.mutate(id, {
                onSuccess: () =>
                  navigate("/music?view=playlists", { replace: true }),
              });
          }}
        >
          <Trash2 size={13} /> {t("Delete")}
        </button>
      </div>
      <div className="music-toolbar-row">
        <PlayButtons load={async () => tracks} disabled={tracks.length === 0} />
      </div>
      {shown.length === 0 ? (
        <EmptyState
          title={
            tracks.length === 0
              ? t("This playlist is empty")
              : t("No song matches")
          }
          icon={<ListMusic size={28} />}
        >
          {tracks.length === 0 && (
            <span>
              {t("Use the … menu on a song and choose Add to playlist.")}
            </span>
          )}
        </EmptyState>
      ) : (
        <TrackList
          tracks={shown}
          onPlay={(track) => usePlayer.getState().play(shown, track.id)}
          extra={(track) => {
            const index = ids.indexOf(track.id);
            return [
              {
                key: "up",
                label: t("Move up"),
                onSelect: () => swap(index, index - 1),
              },
              {
                key: "down",
                label: t("Move down"),
                onSelect: () => swap(index, index + 1),
              },
              {
                key: "remove",
                label: t("Remove from playlist"),
                danger: true,
                onSelect: () => save(ids.filter((x) => x !== track.id)),
              },
            ];
          }}
        />
      )}
      <Dialog
        open={renaming}
        onClose={() => setRenaming(false)}
        title={t("Rename playlist")}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const n = name.trim();
            if (!n) return;
            rename.mutate(
              { id, name: n },
              {
                onSuccess: () => setRenaming(false),
                onError: (err) =>
                  toast({ message: errorMessage(err), tone: "error" }),
              },
            );
          }}
        >
          <div className="xc-field">
            <label htmlFor="music-rename-playlist">{t("Name")}</label>
            <input
              id="music-rename-playlist"
              className="xc-input"
              value={name}
              maxLength={100}
              autoFocus
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="xc-dialog-actions">
            <button
              type="button"
              className="xc-btn"
              onClick={() => setRenaming(false)}
            >
              {t("Cancel")}
            </button>
            <button
              className="xc-btn primary"
              disabled={rename.isPending || !name.trim()}
            >
              {t("Save")}
            </button>
          </div>
        </form>
      </Dialog>
    </>
  );
}
