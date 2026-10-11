import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { apiFetch, createApi, unwrap } from "../../api/client";
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
export type SettingsInput = S["MusicSettingsInput"];
export type Pending = S["MusicPending"];
export type Candidate = S["MusicCandidate"];
export type Sleep = S["MusicSleep"];
export type Share = S["MusicShare"];
export type PublicPlaylist = S["MusicPublicPlaylist"];
export type PublicTrack = S["MusicPublicTrack"];

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
  pending: ["music", "pending"] as const,
  sleep: ["music", "sleep"] as const,
  shares: (playlistId: number) => ["music", "shares", playlistId] as const,
};

// 扫描完成、歌曲改过、播放列表改过时刷新。播放次数的事件不刷新，不然每首歌播放都会重拉所有列表。
invalidateOn("music.library_changed", musicKeys.all);
invalidateOn("music.track_updated", musicKeys.tracks);
invalidateOn("music.playlist_changed", musicKeys.playlists);
invalidateOn("music.sleep_", musicKeys.sleep);

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

/** 只改传了的字段：目录、自动匹配、写回文件、各个来源。 */
export function usePutMusicSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: SettingsInput) =>
      unwrap(musicApi.PUT("/music/settings", { body })),
    onSuccess: refresh,
  });
}

/** 在线匹配没法确定、等用户选的歌。 */
export function usePending() {
  return useQuery({
    queryKey: musicKeys.pending,
    queryFn: () => unwrap(musicApi.GET("/music/pending")),
    retry: false,
  });
}

/** 手动匹配一首歌。传 candidate 是用待确认列表里选的那个；overwrite 是换掉已有的歌词和封面。 */
export function useMatchTrack() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: {
      id: number;
      candidate?: { source: string; sourceId: string };
      overwrite?: boolean;
    }) =>
      unwrap(
        musicApi.POST("/music/tracks/{trackId}/match", {
          params: { path: { trackId: v.id } },
          body: { candidate: v.candidate, overwrite: v.overwrite },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useSkipMatch() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        musicApi.POST("/music/tracks/{trackId}/skip", {
          params: { path: { trackId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 后台给还没匹配过的歌匹配。retryFailed 把以前失败的也重试。 */
export function useMatchAll() {
  return useMutation({
    mutationFn: (retryFailed: boolean) =>
      unwrap(
        musicApi.POST("/music/match", {
          params: { query: { retryFailed: retryFailed || undefined } },
        }),
      ),
  });
}

export function usePutLyrics() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; text: string; writeBack?: boolean }) =>
      unwrap(
        musicApi.PUT("/music/tracks/{trackId}/lyrics", {
          params: { path: { trackId: v.id } },
          body: { text: v.text, writeBack: v.writeBack },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 上传封面图，请求体就是图片内容。 */
export function useUploadCover() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async (v: { id: number; file: File; writeBack?: boolean }) => {
      const q = v.writeBack == null ? "" : `?writeBack=${v.writeBack}`;
      const res = await apiFetch(`/music/tracks/${v.id}/cover${q}`, {
        method: "POST",
        headers: { "Content-Type": "application/octet-stream" },
        body: v.file,
      });
      return (await res.json()) as Track;
    },
    onSuccess: refresh,
  });
}

/** 后台批量匹配的进度，来自 music.match_progress 事件。 */
export interface MatchProgress {
  running: boolean;
  done: number;
  total: number;
  current?: string;
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

/** 定时暂停的状态。放在服务端，所有页面和设备看到的一样。 */
export function useSleep() {
  return useQuery({
    queryKey: musicKeys.sleep,
    queryFn: () => unwrap(musicApi.GET("/music/sleep")),
    retry: false,
    meta: { silentError: true },
  });
}

/** 设定时。三种只能传一个：几分钟后、再播几首、在现有的分钟定时上加几分钟。 */
export function usePutSleep() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: S["MusicSleepInput"]) =>
      unwrap(musicApi.PUT("/music/sleep", { body })),
    onSuccess: (data) => client.setQueryData(musicKeys.sleep, data),
  });
}

export function useCancelSleep() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(musicApi.DELETE("/music/sleep")),
    onSuccess: () => client.setQueryData(musicKeys.sleep, { active: false }),
  });
}

export function useShares(playlistId: number) {
  return useQuery({
    queryKey: musicKeys.shares(playlistId),
    queryFn: () =>
      unwrap(
        musicApi.GET("/music/playlists/{playlistId}/shares", {
          params: { path: { playlistId } },
        }),
      ),
    retry: false,
  });
}

export function useCreateShare(playlistId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: S["MusicShareInput"]) =>
      unwrap(
        musicApi.POST("/music/playlists/{playlistId}/shares", {
          params: { path: { playlistId } },
          body,
        }),
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: musicKeys.shares(playlistId) }),
  });
}

export function useDeleteShare(playlistId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (shareId: number) =>
      unwrap(
        musicApi.DELETE("/music/shares/{shareId}", {
          params: { path: { shareId } },
        }),
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: musicKeys.shares(playlistId) }),
  });
}
