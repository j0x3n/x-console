import { findTrackByDriveItem } from "./api";
import { usePlayer } from "./player";

/**
 * 云盘里点音频文件时，如果这首歌在曲库里，就用全局播放器播放（B147）。
 * 返回 false 表示它不在曲库里（不在音乐目录，或还没扫到），调用方照旧打开预览。
 */
export async function playDriveAudio(driveItemId: number): Promise<boolean> {
  try {
    const track = await findTrackByDriveItem(driveItemId);
    if (!track) return false;
    usePlayer.getState().play([track]);
    return true;
  } catch {
    return false;
  }
}
