import { useQuery } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/backup";

/*
 * 云盘页里浏览备份设置绑定的网盘（B68）：坚果云等 WebDAV 和 Google Drive。
 * 接口在备份模块里，只能浏览和下载。
 */
const api = createApi<paths>();

type S = components["schemas"];
export type RemoteDrive = S["RemoteDrive"];
export type RemoteDriveEntry = S["RemoteDriveEntry"];
export type RemoteId = RemoteDrive["id"];

export const remoteKeys = {
  all: ["remote-drives"] as const,
  items: (remote: RemoteId, ref: string) =>
    ["remote-drives", remote, ref] as const,
};

/** 绑定了的网盘。接口没上线或出错时当作没有，云盘页照常。 */
export function useRemoteDrives() {
  return useQuery({
    queryKey: remoteKeys.all,
    queryFn: () => unwrap(api.GET("/remote-drives")),
    retry: false,
    staleTime: 60_000,
    meta: { silentError: true },
  });
}

export function useRemoteItems(remote: RemoteId, ref: string) {
  return useQuery({
    queryKey: remoteKeys.items(remote, ref),
    queryFn: () =>
      unwrap(
        api.GET("/remote-drives/{remote}/items", {
          params: { path: { remote }, query: ref ? { ref } : {} },
        }),
      ),
    retry: false,
  });
}

export function remoteDownloadUrl(remote: RemoteId, ref: string) {
  return `/api/v1/remote-drives/${remote}/download?ref=${encodeURIComponent(ref)}`;
}
