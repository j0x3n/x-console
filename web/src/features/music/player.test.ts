// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import type { Track } from "./api";
import { loadSaved, persist, usePlayer } from "./player";

const track = (id: number, extra: Partial<Track> = {}): Track => ({
  id,
  driveItemId: id + 1000,
  title: `歌${id}`,
  artist: "歌手",
  album: "专辑",
  albumArtist: "",
  trackNo: 0,
  discNo: 0,
  year: 0,
  durationMs: 180000,
  bitrate: 128,
  format: "mp3",
  hasCover: false,
  coverSource: "none",
  lyricsSource: "none",
  lyricsSynced: false,
  favorite: false,
  playCount: 0,
  matchState: "none",
  createdAt: "2026-10-11T00:00:00Z",
  updatedAt: "2026-10-11T00:00:00Z",
  ...extra,
});

const ids = () => usePlayer.getState().queue.map((t) => t.id);

beforeEach(() => {
  localStorage.clear();
  usePlayer.setState({
    queue: [],
    currentId: null,
    playing: false,
    blocked: false,
    mode: "sequence",
    seed: 1,
    fullOpen: false,
  });
});

describe("播放器状态", () => {
  it("play 换队列并从指定的歌开始", () => {
    usePlayer.getState().play([track(1), track(2), track(3)], 2);
    const s = usePlayer.getState();
    expect(ids()).toEqual([1, 2, 3]);
    expect(s.currentId).toBe(2);
    expect(s.playing).toBe(true);
  });
  it("指定的歌不在列表里时从第一首开始，空列表什么也不做", () => {
    usePlayer.getState().play([track(1), track(2)], 99);
    expect(usePlayer.getState().currentId).toBe(1);
    usePlayer.getState().play([]);
    expect(ids()).toEqual([1, 2]);
  });
  it("next 和 prev 跟着队列走，顺序播放到头就停", () => {
    const s = usePlayer.getState();
    s.play([track(1), track(2)], 1);
    usePlayer.getState().next("manual");
    expect(usePlayer.getState().currentId).toBe(2);
    usePlayer.getState().next("ended");
    expect(usePlayer.getState().playing).toBe(false);
    usePlayer.getState().prev();
    expect(usePlayer.getState().currentId).toBe(1);
  });
  it("下一首播放和加入队列", () => {
    const s = usePlayer.getState();
    s.play([track(1), track(2)], 1);
    usePlayer.getState().playNext([track(9)]);
    expect(ids()).toEqual([1, 9, 2]);
    usePlayer.getState().enqueue([track(2), track(7)]);
    expect(ids()).toEqual([1, 9, 2, 7]);
  });
  it("队列是空时，下一首播放直接开始播", () => {
    usePlayer.getState().playNext([track(5)]);
    expect(usePlayer.getState().currentId).toBe(5);
    expect(usePlayer.getState().playing).toBe(true);
  });
  it("删掉正在播的歌，接着播下一首；删光后停止", () => {
    usePlayer.getState().play([track(1), track(2)], 1);
    usePlayer.getState().removeFromQueue(1);
    expect(usePlayer.getState().currentId).toBe(2);
    usePlayer.getState().removeFromQueue(2);
    expect(usePlayer.getState().currentId).toBeNull();
    expect(usePlayer.getState().playing).toBe(false);
  });
  it("拖动排序和清空", () => {
    usePlayer.getState().play([track(1), track(2), track(3)], 1);
    usePlayer.getState().moveInQueue(0, 2);
    expect(ids()).toEqual([2, 3, 1]);
    usePlayer.getState().clearQueue();
    expect(ids()).toEqual([]);
    expect(usePlayer.getState().currentId).toBeNull();
  });
  it("没有歌时暂停和继续不会进入播放状态", () => {
    usePlayer.getState().resume();
    expect(usePlayer.getState().playing).toBe(false);
    usePlayer.getState().toggle();
    expect(usePlayer.getState().playing).toBe(false);
  });
  it("浏览器拒绝自动播放时标记 blocked 并停在暂停", () => {
    usePlayer.getState().play([track(1)]);
    usePlayer.getState().setBlocked(true);
    expect(usePlayer.getState().blocked).toBe(true);
    expect(usePlayer.getState().playing).toBe(false);
    usePlayer.getState().resume();
    expect(usePlayer.getState().blocked).toBe(false);
  });
  it("patchTrack 同步队列里的那一首（比如收藏）", () => {
    usePlayer.getState().play([track(1), track(2)], 1);
    usePlayer.getState().patchTrack(track(2, { favorite: true }));
    expect(usePlayer.getState().queue[1].favorite).toBe(true);
    usePlayer.getState().patchTrack(track(99));
    expect(ids()).toEqual([1, 2]);
  });
  it("音量限制在 0 到 1", () => {
    usePlayer.getState().setVolume(3);
    expect(usePlayer.getState().volume).toBe(1);
    usePlayer.getState().setVolume(-1);
    expect(usePlayer.getState().volume).toBe(0);
  });
});

describe("记住上次的播放状态", () => {
  it("写入后能读回来，而且读回来是暂停的", () => {
    usePlayer.getState().play([track(1), track(2)], 2);
    usePlayer.getState().setMode("loop");
    usePlayer.getState().setRate(1.5);
    persist(42);
    const saved = loadSaved();
    expect(saved?.queue.map((t) => t.id)).toEqual([1, 2]);
    expect(saved?.currentId).toBe(2);
    expect(saved?.mode).toBe("loop");
    expect(saved?.rate).toBe(1.5);
    expect(saved?.time).toBe(42);
  });
  it("存的内容坏了或不合规就当没有", () => {
    localStorage.setItem("xc.music.player", "{坏的");
    expect(loadSaved()).toBeNull();
    localStorage.setItem(
      "xc.music.player",
      JSON.stringify({
        queue: [{ id: 1, title: "a" }, { nope: true }, null],
        currentId: 77,
        mode: "bogus",
        volume: 9,
        rate: "x",
        time: -5,
      }),
    );
    const saved = loadSaved();
    expect(saved?.queue.map((t) => t.id)).toEqual([1]);
    expect(saved?.currentId).toBeNull();
    expect(saved?.mode).toBe("sequence");
    expect(saved?.volume).toBe(0.8);
    expect(saved?.rate).toBe(1);
    expect(saved?.time).toBe(0);
  });
  it("没有存过时返回 null", () => {
    expect(loadSaved()).toBeNull();
  });
});
