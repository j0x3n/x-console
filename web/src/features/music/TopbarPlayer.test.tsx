// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Track } from "./api";
import { usePlayer } from "./player";
import TopbarPlayer, { useTopbarPlayer } from "./TopbarPlayer";

// 队列和歌词要请求接口，这里只看小组件开合，换成占位。
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

beforeEach(() => {
  localStorage.clear();
  useTopbarPlayer.setState({ open: null, pinned: [] });
  usePlayer.setState({
    queue: [track],
    currentId: 1,
    playing: false,
    blocked: false,
  });
});
afterEach(cleanup);

describe("页头迷你播放器", () => {
  it("没有歌时只有一个去音乐页的入口", () => {
    usePlayer.setState({ queue: [], currentId: null });
    renderBar();
    expect(screen.getByRole("link", { name: "音乐" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "歌词" })).toBeNull();
  });

  it("点歌词在旁边弹出小组件，再点收起", () => {
    renderBar();
    fireEvent.click(screen.getByRole("button", { name: "歌词" }));
    expect(screen.getByText("歌词内容")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "歌词" }));
    expect(screen.queryByText("歌词内容")).toBeNull();
  });

  it("点别处收起没固定的小组件", () => {
    renderBar();
    fireEvent.click(screen.getByRole("button", { name: "队列" }));
    expect(screen.getByText("队列内容")).toBeTruthy();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByText("队列内容")).toBeNull();
  });

  it("固定后点别处不收起，并且记住", () => {
    renderBar();
    fireEvent.click(screen.getByRole("button", { name: "歌词" }));
    fireEvent.click(
      screen.getByRole("button", { name: "固定，所有页面都显示" }),
    );
    fireEvent.pointerDown(document.body);
    expect(screen.getByText("歌词内容")).toBeTruthy();
    expect(JSON.parse(localStorage.getItem("xc.music.pinned") ?? "[]")).toEqual(
      ["lyrics"],
    );
    // 固定的再点按钮，取消固定并收起
    fireEvent.click(screen.getByRole("button", { name: "歌词" }));
    expect(screen.queryByText("歌词内容")).toBeNull();
    expect(useTopbarPlayer.getState().pinned).toEqual([]);
  });

  it("播放暂停按钮切换播放状态", () => {
    renderBar();
    fireEvent.click(screen.getByRole("button", { name: "播放" }));
    expect(usePlayer.getState().playing).toBe(true);
  });
});
