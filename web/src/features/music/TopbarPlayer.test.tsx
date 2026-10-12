// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Track } from "./api";
import { usePlayer } from "./player";
import { syncPosition } from "./PlayerHost";
import TopbarPlayer, { useTopbarPlayer } from "./TopbarPlayer";

// 队列和歌词要请求接口，这里只看开合，换成占位。
vi.mock("./LyricsView", () => ({ default: () => <div>歌词内容</div> }));
vi.mock("./QueueList", () => ({ default: () => <div>队列内容</div> }));
vi.mock("./SleepDialog", () => ({ SleepButton: () => null }));

const track: Track = {
  id: 1,
  driveItemId: 1001,
  title: "夜曲",
  artist: "周杰伦",
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
};

function renderBar() {
  return render(
    <MemoryRouter>
      <TopbarPlayer />
    </MemoryRouter>,
  );
}

function resetMini(
  patch: Partial<ReturnType<typeof useTopbarPlayer.getState>> = {},
) {
  useTopbarPlayer.setState({
    expanded: false,
    open: false,
    pinned: false,
    queueOpen: false,
    ...patch,
  });
}

beforeEach(() => {
  localStorage.clear();
  resetMini();
  usePlayer.setState({
    queue: [track],
    currentId: 1,
    playing: false,
    blocked: false,
  });
});
afterEach(cleanup);

const cover = () => screen.getByRole("button", { name: "正在播放: 夜曲" });

describe("页头播放器", () => {
  it("没有歌时只有一个去音乐页的入口", () => {
    usePlayer.setState({ queue: [], currentId: null });
    renderBar();
    expect(screen.getByRole("link", { name: "音乐" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "下一首" })).toBeNull();
  });

  it("默认只有播放按钮，点了开始播放并展开", () => {
    renderBar();
    expect(useTopbarPlayer.getState().expanded).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "播放" }));
    expect(usePlayer.getState().playing).toBe(true);
    expect(useTopbarPlayer.getState().expanded).toBe(true);
    expect(screen.getByRole("button", { name: "暂停" })).toBeTruthy();
    expect(
      JSON.parse(localStorage.getItem("xc.music.mini") ?? "{}"),
    ).toMatchObject({
      expanded: true,
    });
    // 收起后只剩一个按钮，播放不停
    fireEvent.click(screen.getByRole("button", { name: "收起播放器" }));
    expect(useTopbarPlayer.getState().expanded).toBe(false);
    expect(usePlayer.getState().playing).toBe(true);
    expect(screen.getByRole("button", { name: "展开播放器" })).toBeTruthy();
  });

  it("点封面弹出迷你播放器，带歌词，点别处收起", () => {
    resetMini({ expanded: true });
    renderBar();
    fireEvent.click(cover());
    expect(screen.getByRole("dialog", { name: "正在播放" })).toBeTruthy();
    expect(screen.getByText("歌词内容")).toBeTruthy();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("dialog", { name: "正在播放" })).toBeNull();
  });

  it("迷你播放器里展开和收起播放列表", () => {
    resetMini({ expanded: true });
    renderBar();
    fireEvent.click(cover());
    expect(screen.queryByText("队列内容")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "队列" }));
    expect(screen.getByText("队列内容")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "队列" }));
    expect(screen.queryByText("队列内容")).toBeNull();
  });

  it("固定后点别处不收起，并且记住", () => {
    resetMini({ expanded: true });
    renderBar();
    fireEvent.click(cover());
    fireEvent.click(
      screen.getByRole("button", { name: "固定，所有页面都显示" }),
    );
    fireEvent.pointerDown(document.body);
    expect(screen.getByText("歌词内容")).toBeTruthy();
    expect(
      JSON.parse(localStorage.getItem("xc.music.mini") ?? "{}"),
    ).toMatchObject({
      pinned: true,
    });
    // 固定的再点封面，取消固定并收起
    fireEvent.click(cover());
    expect(screen.queryByText("歌词内容")).toBeNull();
    expect(useTopbarPlayer.getState().pinned).toBe(false);
  });

  it("展开后的暂停和下一首", () => {
    resetMini({ expanded: true });
    usePlayer.setState({ playing: true });
    renderBar();
    fireEvent.click(screen.getByRole("button", { name: "暂停" }));
    expect(usePlayer.getState().playing).toBe(false);
  });
});

describe("系统媒体控制的进度", () => {
  it("按当前位置和时长更新，没有时长时清空", () => {
    const setPositionState = vi.fn();
    Object.defineProperty(navigator, "mediaSession", {
      configurable: true,
      value: { setPositionState },
    });
    const el = document.createElement("audio");
    Object.defineProperty(el, "duration", { configurable: true, value: 200 });
    el.currentTime = 42;
    syncPosition(el);
    expect(setPositionState).toHaveBeenLastCalledWith({
      duration: 200,
      position: 42,
      playbackRate: 1,
    });
    // 换歌后还没读到时长
    Object.defineProperty(el, "duration", { configurable: true, value: NaN });
    syncPosition(el);
    expect(setPositionState).toHaveBeenLastCalledWith();
    Reflect.deleteProperty(navigator, "mediaSession");
  });

  it("音量只有一个按钮，点开出竖直滑块，滚轮也能调", () => {
    resetMini({ expanded: true, open: true });
    usePlayer.setState({ volume: 0.5 });
    renderBar();
    expect(screen.queryByRole("slider", { name: "音量" })).toBeNull();
    const button = screen.getByRole("button", { name: "音量" });
    fireEvent.wheel(button, { deltaY: -100 });
    expect(usePlayer.getState().volume).toBeCloseTo(0.55);
    fireEvent.click(button);
    const slider = screen.getByRole("slider", { name: "音量" });
    fireEvent.change(slider, { target: { value: "20" } });
    expect(usePlayer.getState().volume).toBeCloseTo(0.2);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("slider", { name: "音量" })).toBeNull();
  });
});
