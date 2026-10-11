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
import ShareDialog from "./ShareDialog";
import SleepDialog from "./SleepDialog";
import MusicSharePage from "./share/MusicSharePage";
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
  sessionStorage.clear();
});

describe("定时暂停弹窗", () => {
  it("点 30 分钟发 minutes: 30", async () => {
    api.routes.set("GET /music/sleep", () => ({
      status: 200,
      body: { active: false },
    }));
    api.routes.set("PUT /music/sleep", () => ({
      status: 200,
      body: { active: true, mode: "time", endsAt: "2099-01-01T00:00:00Z" },
    }));
    const onClose = vi.fn();
    wrap(<SleepDialog onClose={onClose} />);
    fireEvent.click(await screen.findByRole("button", { name: "30 分钟" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      minutes: 30,
    });
  });

  it("播完这首发 tracks: 1，自定义首数要在 2 到 20 之间", async () => {
    api.routes.set("GET /music/sleep", () => ({
      status: 200,
      body: { active: false },
    }));
    api.routes.set("PUT /music/sleep", () => ({
      status: 200,
      body: { active: true, mode: "tracks", tracksLeft: 1 },
    }));
    wrap(<SleepDialog onClose={() => {}} />);
    const startButtons = await screen.findAllByRole("button", {
      name: "开始定时",
    });
    fireEvent.change(screen.getByLabelText("歌曲"), { target: { value: "1" } });
    expect((startButtons[1] as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("歌曲"), { target: { value: "5" } });
    expect((startButtons[1] as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "播完这首" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        tracks: 1,
      }),
    );
  });

  it("已经有定时时显示剩余并能取消、延长", async () => {
    api.routes.set("GET /music/sleep", () => ({
      status: 200,
      body: {
        active: true,
        mode: "time",
        endsAt: new Date(Date.now() + 600_000).toISOString(),
      },
    }));
    api.routes.set("PUT /music/sleep", () => ({
      status: 200,
      body: {
        active: true,
        mode: "time",
        endsAt: new Date(Date.now() + 1_500_000).toISOString(),
      },
    }));
    api.routes.set("DELETE /music/sleep", () => ({ status: 204 }));
    wrap(<SleepDialog onClose={() => {}} />);
    expect(await screen.findByText(/还剩 \d\d:\d\d/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "再加 15 分钟" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        extendMinutes: 15,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "取消定时" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });
});

describe("分享弹窗", () => {
  it("生成带密码和有效期的链接，列出已有的链接", async () => {
    api.routes.set("GET /music/playlists/7/shares", () => ({
      status: 200,
      body: [
        {
          id: 1,
          playlistId: 7,
          path: "/m/abc",
          hasPassword: true,
          viewCount: 3,
          createdAt: "2026-10-11T00:00:00Z",
        },
      ],
    }));
    api.routes.set("POST /music/playlists/7/shares", () => ({
      status: 201,
      body: {
        id: 2,
        playlistId: 7,
        path: "/m/def",
        hasPassword: true,
        viewCount: 0,
        createdAt: "2026-10-11T00:00:00Z",
      },
    }));
    wrap(<ShareDialog playlistId={7} name="跑步" onClose={() => {}} />);
    expect(await screen.findByText(/\/m\/abc/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("密码（可不填）"), {
      target: { value: "秘密" },
    });
    fireEvent.change(screen.getByLabelText("有效期"), {
      target: { value: "7" },
    });
    fireEvent.click(screen.getByRole("button", { name: /生成链接/ }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        password: "秘密",
        expiresInDays: 7,
      }),
    );
  });

  it("撤销链接调用删除接口", async () => {
    api.routes.set("GET /music/playlists/7/shares", () => ({
      status: 200,
      body: [
        {
          id: 1,
          playlistId: 7,
          path: "/m/abc",
          hasPassword: false,
          viewCount: 0,
          createdAt: "2026-10-11T00:00:00Z",
        },
      ],
    }));
    api.routes.set("DELETE /music/shares/1", () => ({ status: 204 }));
    wrap(<ShareDialog playlistId={7} name="跑步" onClose={() => {}} />);
    fireEvent.click(await screen.findByRole("button", { name: /撤销链接/ }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });
});

describe("公开的分享页", () => {
  const playlist = {
    name: "跑步",
    tracks: [
      {
        id: 1,
        title: "夜曲",
        artist: "周杰伦",
        album: "",
        durationMs: 226000,
        hasCover: false,
        hasLyrics: false,
      },
    ],
  };

  it("没有密码时直接列出歌", async () => {
    api.routes.set("GET /public/music/tok", () => ({
      status: 200,
      body: playlist,
    }));
    wrap(<MusicSharePage token="tok" />);
    expect(await screen.findByText("夜曲")).toBeTruthy();
    expect(screen.queryByLabelText("密码")).toBeNull();
  });

  it("有密码时先输密码，输对后用访问令牌取列表", async () => {
    api.routes.set("GET /public/music/tok", () => {
      const last = api.calls[api.calls.length - 1].path;
      return last.includes("access=ok-access")
        ? { status: 200, body: playlist }
        : {
            status: 401,
            body: { code: "share_code_required", message: "请输入密码" },
          };
    });
    api.routes.set("POST /public/music/tok/unlock", () => ({
      status: 200,
      body: { access: "ok-access", expiresAt: "2099-01-01T00:00:00Z" },
    }));
    wrap(<MusicSharePage token="tok" />);
    const input = await screen.findByLabelText("密码");
    fireEvent.change(input, { target: { value: "秘密" } });
    fireEvent.click(screen.getByRole("button", { name: "打开" }));
    expect(await screen.findByText("夜曲")).toBeTruthy();
    expect(api.calls.find((c) => c.path.endsWith("/unlock"))?.body).toEqual({
      code: "秘密",
    });
    expect(sessionStorage.getItem("xc.music-share.tok")).toBe("ok-access");
  });

  it("密码不对时显示原因，链接失效时显示失效", async () => {
    api.routes.set("GET /public/music/tok", () => ({
      status: 401,
      body: { code: "share_code_required", message: "请输入密码" },
    }));
    api.routes.set("POST /public/music/tok/unlock", () => ({
      status: 403,
      body: { code: "share_code_invalid", message: "密码不对" },
    }));
    wrap(<MusicSharePage token="tok" />);
    fireEvent.change(await screen.findByLabelText("密码"), {
      target: { value: "x" },
    });
    fireEvent.click(screen.getByRole("button", { name: "打开" }));
    expect(await screen.findByText("密码不对")).toBeTruthy();
    cleanup();
    api.routes.clear();
    wrap(<MusicSharePage token="gone" />);
    expect(await screen.findByText("这个链接不存在或已失效")).toBeTruthy();
  });
});
