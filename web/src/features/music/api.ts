import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/music";

/* 音乐曲库（B146 后端，B147 前端）。歌在云盘里，这里是它们的索引、封面和歌词。 */
export const musicApi = createApi<paths>();

type S = components["schemas"];
export type Track = S["MusicTrack"];
export type TrackSort = NonNullable<
  paths["/music/tracks"]["get"]["parameters"]["query"]
>["sort"];
export type Album = S["MusicAlbum"];
export type Artist = S["MusicArtist"];
export type Playlist = S["MusicPlaylist"];
export type PlaylistDetail = S["MusicPlaylistDetail"];
export type MusicSettings = S["MusicSettings"];
export type Lyrics = S["MusicLyrics"];

export const musicKeys = {
  all: ["music"] as const,
  tracks: ["music", "tracks"] as const,
  list: (f: TrackFilter) => ["music", "tracks", f] as const,
  albums: ["music", "albums"] as const,
  artists: ["music", "artists"] as const,
  playlists: ["music", "playlists"] as const,
  playlist: (id: number) => ["music", "playlists", id] as const,
  lyrics: (id: number) => ["music", "lyrics", id] as const,
  settings: ["music", "settings"] as const,
};

// 扫描完成、歌曲改过、播放列表改过时刷新。播放次数的事件不刷新，不然每首歌播放都会重拉所有列表。
invalidateOn("music.library_changed", musicKeys.all);
invalidateOn("music.track_updated", musicKeys.tracks);
invalidateOn("music.playlist_changed", musicKeys.playlists);

export function streamUrl(id: number): string {
  return `/api/v1/music/tracks/${id}/stream`;
}

/** 封面地址。updatedAt 变了说明封面可能换了，让浏览器重新取。 */
export function coverUrl(
  id: number,
  size: 96 | 256 | 640 = 256,
  version?: string,
): string {
  const v = version ? `&v=${encodeURIComponent(version)}` : "";
  return `/api/v1/music/tracks/${id}/cover?size=${size}${v}`;
}

export interface TrackFilter {
  q?: string;
  artist?: string;
  album?: string;
  favorite?: boolean;
  sort?: TrackSort;
}

const PAGE = 200;

/** 歌曲列表，分页拉取，调用 fetchNextPage 加载更多。items 是已经拉到的全部。 */
export function useTracks(filter: TrackFilter) {
  const query = useInfiniteQuery({
    queryKey: musicKeys.list(filter),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      unwrap(
        musicApi.GET("/music/tracks", {
          params: {
            query: {
              q: filter.q || undefined,
              artist: filter.artist,
              album: filter.album,
              favorite: filter.favorite || undefined,
              sort: filter.sort,
              limit: PAGE,
              cursor: pageParam,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.nextCursor,
    placeholderData: keepPreviousData,
    retry: false,
  });
  const pages = query.data?.pages ?? [];
  return {
    ...query,
    items: pages.flatMap((p) => p.items),
    total: pages[0]?.total ?? 0,
  };
}

/** 这个云盘文件对应的歌。没在曲库里时返回 null。 */
export async function findTrackByDriveItem(
  driveItemId: number,
): Promise<Track | null> {
  const page = await unwrap(
    musicApi.GET("/music/tracks", {
      params: { query: { driveItemId, limit: 1 } },
    }),
  );
  return page.items[0] ?? null;
}

export function useAlbums() {
  return useQuery({
    queryKey: musicKeys.albums,
    queryFn: () => unwrap(musicApi.GET("/music/albums")),
    retry: false,
  });
}

export function useArtists() {
  return useQuery({
    queryKey: musicKeys.artists,
    queryFn: () => unwrap(musicApi.GET("/music/artists")),
    retry: false,
  });
}

export function usePlaylists() {
  return useQuery({
    queryKey: musicKeys.playlists,
    queryFn: () => unwrap(musicApi.GET("/music/playlists")),
    retry: false,
  });
}

export function usePlaylist(id: number | null) {
  return useQuery({
    queryKey: musicKeys.playlist(id ?? 0),
    queryFn: () =>
      unwrap(
        musicApi.GET("/music/playlists/{playlistId}", {
          params: { path: { playlistId: id ?? 0 } },
        }),
      ),
    enabled: id != null,
    retry: false,
  });
}

/** 歌词。歌换了才重新取，不会自己过期。 */
export function useLyrics(trackId: number | null) {
  return useQuery({
    queryKey: musicKeys.lyrics(trackId ?? 0),
    queryFn: () =>
      unwrap(
        musicApi.GET("/music/tracks/{trackId}/lyrics", {
          params: { path: { trackId: trackId ?? 0 } },
        }),
      ),
    enabled: trackId != null,
    staleTime: 5 * 60 * 1000,
  });
}

export function useMusicSettings() {
  return useQuery({
    queryKey: musicKeys.settings,
    queryFn: () => unwrap(musicApi.GET("/music/settings")),
    retry: false,
  });
}

function useRefresh() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: musicKeys.all });
}

export function usePutMusicSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (folders: number[]) =>
      unwrap(musicApi.PUT("/music/settings", { body: { folders } })),
    onSuccess: refresh,
  });
}

export function useScanMusic() {
  return useMutation({
    mutationFn: () => unwrap(musicApi.POST("/music/scan")),
  });
}

export function useUpdateTrack() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; patch: NonNullable<S["MusicTrackPatch"]> }) =>
      unwrap(
        musicApi.PATCH("/music/tracks/{trackId}", {
          params: { path: { trackId: v.id } },
          body: v.patch,
        }),
      ),
    onSuccess: refresh,
  });
}

/** 播放超过 30 秒或一半时上报一次。失败不提示，不影响播放。 */
export function reportPlayed(id: number, seconds: number) {
  void musicApi
    .POST("/music/tracks/{trackId}/played", {
      params: { path: { trackId: id } },
      body: { seconds: Math.max(0, Math.round(seconds)) },
    })
    .catch(() => undefined);
}

export function useCreatePlaylist() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { name: string; trackIds?: number[] }) =>
      unwrap(musicApi.POST("/music/playlists", { body: v })),
    onSuccess: refresh,
  });
}

export function useRenamePlaylist() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; name: string }) =>
      unwrap(
        musicApi.PATCH("/music/playlists/{playlistId}", {
          params: { path: { playlistId: v.id } },
          body: { name: v.name },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeletePlaylist() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        musicApi.DELETE("/music/playlists/{playlistId}", {
          params: { path: { playlistId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 往播放列表末尾加歌，已经在里面的跳过。 */
export function useAddToPlaylist() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; trackIds: number[] }) =>
      unwrap(
        musicApi.POST("/music/playlists/{playlistId}/items", {
          params: { path: { playlistId: v.id } },
          body: { trackIds: v.trackIds },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 整体设置播放列表里的歌和顺序。 */
export function useSetPlaylistItems() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; trackIds: number[] }) =>
      unwrap(
        musicApi.PUT("/music/playlists/{playlistId}/items", {
          params: { path: { playlistId: v.id } },
          body: { trackIds: v.trackIds },
        }),
      ),
    onSuccess: refresh,
  });
}
