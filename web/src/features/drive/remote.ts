/*
 * 云盘页里浏览网盘账号（B68、B69）：坚果云等 WebDAV 和 Google Drive。
 * 账号在 设置 → 存储 里管，这里只能浏览和下载。
 */
export {
  remoteDownloadUrl,
  useDriveRemotes as useRemoteDrives,
  useRemoteItems,
  type StorageRemote as RemoteDrive,
  type StorageRemoteEntry as RemoteDriveEntry,
} from "../storage/api";
