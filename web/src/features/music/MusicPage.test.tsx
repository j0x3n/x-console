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
import MusicPage from "./MusicPage";
import MusicSettingsTab from "./MusicSettingsTab";
import MusicDialogs from "./dialogs";
import { usePlayer } from "./player";
import "./i18n";

function renderIt(url = "/music") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <MusicPage />
        <MusicDialogs />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const song = (id: number, title: string, extra: object = {}) => ({
  id,
  driveItemId: id + 100,
  title,
  artist: "周杰伦",
  album: "十一月的萧邦",
  albumArtist: "",
  trackNo: id,
  discNo: 1,
  year: 2005,
  durationMs: 226000,
  bitrate: 320,
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

function setup(folders: unknown[], items: unknown[]) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /music/settings", () => ({
    status: 200,
    body: { folders },
  }));
  api.routes.set("GET /music/tracks", () => ({
    status: 200,
    body: { items, total: items.length },
  }));
  api.routes.set("GET /music/albums", () => ({ status: 200, body: [] }));
  api.routes.set("GET /music/artists", () => ({ status: 200, body: [] }));
  api.routes.set("GET /music/playlists", () => ({
    status: 200,
    body: [
      {
        id: 7,
        name: "跑步",
        trackCount: 0,
        durationMs: 0,
        coverTrackIds: [],
        createdAt: "2026-10-11T00:00:00Z",
        updatedAt: "2026-10-11T00:00:00Z",
      },
    ],
  }));
}

afterEach(() => {
  cleanup();
  usePlayer.setState({ queue: [], currentId: null, playing: false });
});

describe("音乐页", () => {
  it("还没选音乐目录时给出去设置的入口", async () => {
    setup([], []);
    renderIt();
    expect(await screen.findByText("还没有音乐目录")).toBeTruthy();
    const link = screen.getByRole("link", { name: /选择音乐目录/ });
    expect(link.getAttribute("href")).toBe("/settings/music");
  });

  it("接口还没上线时显示还没上线，不显示出错", async () => {
    api.routes.clear();
    renderIt();
    await waitFor(() => expect(screen.queryByText("出错了")).toBeNull());
    expect(await screen.findByText(/还没上线|音乐/)).toBeTruthy();
  });

  it("列出歌，点封面上的播放按钮把整个列表放进队列并从这首开始", async () => {
    setup(
      [{ id: 1, name: "音乐", path: "/音乐" }],
      [song(1, "夜曲"), song(2, "七里香")],
    );
    renderIt();
    expect(await screen.findByText("夜曲")).toBeTruthy();
    expect(screen.getByText("七里香")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "播放 七里香" }));
    const s = usePlayer.getState();
    expect(s.queue.map((t) => t.id)).toEqual([1, 2]);
    expect(s.currentId).toBe(2);
    expect(s.playing).toBe(true);
  });

  it("播放全部从第一首开始；随机播放把模式设成随机", async () => {
    setup(
      [{ id: 1, name: "音乐", path: "/音乐" }],
      [song(1, "夜曲"), song(2, "七里香")],
    );
    renderIt();
    await screen.findByText("夜曲");
    fireEvent.click(screen.getByRole("button", { name: /播放全部/ }));
    await waitFor(() => expect(usePlayer.getState().currentId).toBe(1));
    expect(usePlayer.getState().mode).toBe("sequence");
    fireEvent.click(screen.getByRole("button", { name: /随机播放/ }));
    await waitFor(() => expect(usePlayer.getState().mode).toBe("shuffle"));
  });

  it("点心形按钮收藏这首歌", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], [song(1, "夜曲")]);
    api.routes.set("PATCH /music/tracks/1", (body) => ({
      status: 200,
      body: song(1, "夜曲", body as object),
    }));
    renderIt();
    await screen.findByText("夜曲");
    fireEvent.click(screen.getByRole("button", { name: "加入收藏" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        favorite: true,
      }),
    );
  });

  it("没有符合的歌和空列表有各自的提示", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], []);
    renderIt("/music?view=favorites");
    expect(await screen.findByText("还没有收藏的歌")).toBeTruthy();
  });

  it("搜索框的内容会带给接口", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], [song(1, "夜曲")]);
    renderIt();
    await screen.findByText("夜曲");
    fireEvent.change(screen.getByLabelText("搜索歌名、歌手、专辑和歌词"), {
      target: { value: "夜曲" },
    });
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.includes("q=%E5%A4%9C%E6%9B%B2")),
      ).toBe(true),
    );
  });

  it("播放列表页可以新建播放列表", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], []);
    api.routes.set("POST /music/playlists", (body) => ({
      status: 201,
      body: {
        id: 8,
        name: (body as { name: string }).name,
        trackCount: 0,
        durationMs: 0,
        coverTrackIds: [],
        createdAt: "2026-10-11T00:00:00Z",
        updatedAt: "2026-10-11T00:00:00Z",
        tracks: [],
      },
    }));
    renderIt("/music?view=playlists");
    expect(await screen.findByText("跑步")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /新建播放列表/ }));
    fireEvent.change(await screen.findByLabelText("名称"), {
      target: { value: "睡前" },
    });
    fireEvent.click(screen.getByRole("button", { name: "新建" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        name: "睡前",
      }),
    );
  });

  it("待确认：采用一个候选会把来源和编号发给接口", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], []);
    api.routes.set("GET /music/pending", () => ({
      status: 200,
      body: [
        {
          track: song(1, "夜曲", { matchState: "pending" }),
          candidates: [
            {
              source: "netease",
              sourceId: "99",
              title: "夜曲",
              artist: "周杰伦",
              album: "十一月的萧邦",
              durationMs: 236000,
              lyrics: true,
              cover: true,
            },
          ],
        },
      ],
    }));
    api.routes.set("POST /music/tracks/1/match", () => ({
      status: 200,
      body: song(1, "夜曲", { matchState: "matched" }),
    }));
    renderIt("/music?view=pending");
    expect(await screen.findByText("网易云音乐")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /采用/ }));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/music/tracks/1/match")?.body,
      ).toEqual({ candidate: { source: "netease", sourceId: "99" } }),
    );
  });

  it("待确认：没有要确认的歌时有说明", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], []);
    api.routes.set("GET /music/pending", () => ({ status: 200, body: [] }));
    renderIt("/music?view=pending");
    expect(await screen.findByText("没有要确认的歌")).toBeTruthy();
  });

  it("待确认：都不对会调用跳过", async () => {
    setup([{ id: 1, name: "音乐", path: "/音乐" }], []);
    api.routes.set("GET /music/pending", () => ({
      status: 200,
      body: [
        { track: song(1, "夜曲", { matchState: "pending" }), candidates: [] },
      ],
    }));
    api.routes.set("POST /music/tracks/1/skip", () => ({
      status: 200,
      body: song(1, "夜曲", { matchState: "skipped" }),
    }));
    renderIt("/music?view=pending");
    fireEvent.click(await screen.findByRole("button", { name: /都不对/ }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/music/tracks/1/skip")).toBe(
        true,
      ),
    );
  });
});

describe("设置里的音乐标签", () => {
  const settings = {
    folders: [{ id: 1, name: "音乐", path: "/音乐" }],
    autoMatch: true,
    writeBack: true,
    providers: {
      lrclib: true,
      netease: true,
      qqmusic: true,
      itunes: true,
      musicbrainz: true,
    },
  };
  function renderTab() {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <MusicSettingsTab />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it("关掉写回文件只发这一项", async () => {
    api.routes.clear();
    api.calls.length = 0;
    api.routes.set("GET /music/settings", () => ({
      status: 200,
      body: settings,
    }));
    api.routes.set("GET /music/pending", () => ({ status: 200, body: [] }));
    api.routes.set("PUT /music/settings", (body) => ({
      status: 200,
      body: { ...settings, ...(body as object) },
    }));
    renderTab();
    const toggle = await screen.findByRole("checkbox", {
      name: "把歌词和封面写进歌曲文件",
    });
    expect((toggle as HTMLInputElement).checked).toBe(true);
    fireEvent.click(toggle);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        writeBack: false,
      }),
    );
  });

  it("没有官方接口的来源有提示，单独开关某个来源", async () => {
    api.routes.clear();
    api.calls.length = 0;
    api.routes.set("GET /music/settings", () => ({
      status: 200,
      body: settings,
    }));
    api.routes.set("GET /music/pending", () => ({ status: 200, body: [] }));
    api.routes.set("PUT /music/settings", () => ({
      status: 200,
      body: settings,
    }));
    renderTab();
    expect(
      (await screen.findAllByText("没有官方接口，可能随时失效。")).length,
    ).toBe(2);
    fireEvent.click(screen.getByRole("checkbox", { name: "QQ 音乐" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        providers: { qqmusic: false },
      }),
    );
  });

  it("立即匹配调用批量匹配接口", async () => {
    api.routes.clear();
    api.calls.length = 0;
    api.routes.set("GET /music/settings", () => ({
      status: 200,
      body: settings,
    }));
    api.routes.set("GET /music/pending", () => ({ status: 200, body: [] }));
    api.routes.set("POST /music/match", () => ({
      status: 202,
      body: { running: true },
    }));
    renderTab();
    fireEvent.click(await screen.findByRole("button", { name: /立即匹配/ }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/music/match")).toBe(true),
    );
  });
});
