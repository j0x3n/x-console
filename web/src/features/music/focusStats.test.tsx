// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<string, (body: unknown) => Reply>();
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  const BaseRequest = globalThis.Request;
  globalThis.Request = class extends BaseRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(
        typeof input === "string" && input.startsWith("/")
          ? `http://localhost${input}`
          : input,
        init,
      );
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const req = input as Request;
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path: path + url.search, body });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(body)
      : { status: 404, body: { code: "not_found", message: "not found" } };
    if (reply.body === undefined)
      return new Response(null, { status: reply.status });
    return new Response(JSON.stringify(reply.body), {
      status: reply.status,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return { routes, calls };
});

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
}));
vi.mock("../../components/ui/ConfirmDialog", () => ({
  confirmAction: async () => true,
}));

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import MusicSettingsTab from "./MusicSettingsTab";
import StatsView, { listenText } from "./StatsView";
import TodayMusicCard from "./TodayMusicCard";
import { usePlayer } from "./player";
import "./i18n";

function wrap(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  usePlayer.setState({ queue: [], currentId: null, playing: false });
});

describe("听歌统计", () => {
  it("时长的写法", () => {
    const t = (s: string) =>
      ({ seconds: "秒", minutes: "分钟", hours: "小时" })[s] ?? s;
    expect(listenText(t, 45)).toBe("45 秒");
    expect(listenText(t, 12 * 60)).toBe("12 分钟");
    expect(listenText(t, 3900)).toBe("1 小时 5 分钟");
    expect(listenText(t, 7200)).toBe("2 小时");
  });

  it("显示总时长、专注期间和排行，只给数字", async () => {
    api.routes.set("GET /music/stats", () => ({
      status: 200,
      body: {
        days: 30,
        totalSeconds: 3900,
        plays: 12,
        focusSeconds: 600,
        focusPlays: 3,
        topTracks: [
          {
            trackId: 1,
            title: "夜曲",
            artist: "周杰伦",
            plays: 5,
            seconds: 900,
          },
        ],
        topArtists: [{ artist: "周杰伦", plays: 7, seconds: 1500 }],
        focusTopTracks: [],
      },
    }));
    wrap(<StatsView />);
    expect(await screen.findByText("1 小时 5 分钟")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("夜曲")).toBeTruthy();
    expect(screen.getByText(/5 次 · 15 分钟/)).toBeTruthy();
    expect(screen.getByText("没有数据")).toBeTruthy(); // 专注期间还没有
    fireEvent.click(screen.getByRole("button", { name: "7 天" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.includes("days=7"))).toBe(true),
    );
  });

  it("这段时间没听歌时有说明", async () => {
    api.routes.set("GET /music/stats", () => ({
      status: 200,
      body: {
        days: 30,
        totalSeconds: 0,
        plays: 0,
        focusSeconds: 0,
        focusPlays: 0,
        topTracks: [],
        topArtists: [],
        focusTopTracks: [],
      },
    }));
    wrap(<StatsView />);
    expect(await screen.findByText("这段时间没有听歌")).toBeTruthy();
  });
});

describe("设置里的专注同步", () => {
  const settings = {
    folders: [],
    autoMatch: true,
    writeBack: true,
    providers: { lrclib: true },
    focus: {
      autoPlay: false,
      playlistId: 0,
      autoPause: false,
      onlyFocusList: false,
    },
  };
  function setup() {
    api.routes.set("GET /music/settings", () => ({
      status: 200,
      body: settings,
    }));
    api.routes.set("GET /music/pending", () => ({ status: 200, body: [] }));
    api.routes.set("GET /music/playlists", () => ({
      status: 200,
      body: [
        {
          id: 3,
          name: "专注",
          trackCount: 2,
          durationMs: 1,
          coverTrackIds: [],
          createdAt: "2026-10-11T00:00:00Z",
          updatedAt: "2026-10-11T00:00:00Z",
        },
      ],
    }));
    api.routes.set("PUT /music/settings", () => ({
      status: 200,
      body: settings,
    }));
  }

  it("打开自动播放只发这一项", async () => {
    setup();
    wrap(<MusicSettingsTab />);
    fireEvent.click(
      await screen.findByRole("checkbox", { name: "专注开始时自动播放" }),
    );
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        focus: { autoPlay: true },
      }),
    );
  });

  it("选专注列表；没开自动播放或没选列表时，“只用专注列表”不能开", async () => {
    setup();
    wrap(<MusicSettingsTab />);
    const only = (await screen.findByRole("checkbox", {
      name: "专注期间只用专注列表",
    })) as HTMLInputElement;
    expect(only.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("专注用的播放列表"), {
      target: { value: "3" },
    });
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        focus: { playlistId: 3 },
      }),
    );
  });
});

describe("今日页的正在播放卡片", () => {
  it("没有歌时说明没有在播放，有歌时能播放暂停和切歌", () => {
    wrap(<TodayMusicCard />);
    expect(screen.getByText("没有在播放")).toBeTruthy();
    cleanup();
    const track = {
      id: 1,
      driveItemId: 2,
      title: "夜曲",
      artist: "周杰伦",
      album: "",
      albumArtist: "",
      trackNo: 0,
      discNo: 0,
      year: 0,
      durationMs: 1,
      bitrate: 1,
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
    } as never;
    usePlayer.setState({ queue: [track], currentId: 1, playing: true });
    wrap(<TodayMusicCard />);
    expect(screen.getByText("夜曲")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "暂停" }));
    expect(usePlayer.getState().playing).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "播放" }));
    expect(usePlayer.getState().playing).toBe(true);
  });
});
