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
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import ReadLaterPage from "./ReadLaterPage";
import SharePage from "./SharePage";
import { linkFromShare, parseTags } from "./format";
import "./i18n";

function renderIt(url = "/readlater") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route path="/readlater" element={<ReadLaterPage />} />
          <Route path="/readlater/share" element={<SharePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  excerpt: "",
  note: "",
  source: "web",
  error: "",
  hasContent: true,
  hasHtml: false,
  kind: "page",
  meta: {},
  read: false,
  createdAt: "2026-10-10T01:00:00Z",
  updatedAt: "2026-10-10T01:00:00Z",
  tags: [],
  summary: "",
  site: "example.com",
};
const article = {
  ...base,
  id: 1,
  url: "https://example.com/whale",
  title: "鲸鱼协议详解",
  status: "ready",
  summary: "第一行讲协议。\n第二行讲部署。",
  tags: ["协议", "网络"],
};
const fetching = {
  ...base,
  id: 2,
  url: "https://example.com/new",
  title: "example.com",
  status: "queued",
  hasContent: false,
};
const broken = {
  ...base,
  id: 3,
  url: "https://example.com/gone",
  title: "gone.example",
  status: "failed",
  error: "网页打不开，状态码 404",
  hasContent: false,
};

function serve(items = [article, fetching, broken]) {
  api.routes.set("GET /readlater", () => ({
    status: 200,
    body: {
      items,
      counts: { unread: 3, read: 4, all: 7, failed: 1 },
      tags: [
        { tag: "协议", count: 2 },
        { tag: "网络", count: 1 },
      ],
    },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("ReadLaterPage B117", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("稍后阅读还没上线")).toBeTruthy();
  });

  it("没有链接时引导添加，保存时带上网址和备注", async () => {
    api.routes.set("GET /readlater", () => ({
      status: 200,
      body: {
        items: [],
        counts: { unread: 0, read: 0, all: 0, failed: 0 },
        tags: [],
      },
    }));
    api.routes.set("POST /readlater", () => ({
      status: 201,
      body: { item: article, duplicate: false },
    }));
    renderIt();
    expect(await screen.findByText("还没有存过链接")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /添加链接/ })[0]!);
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("请填写网址")).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("网址"), {
      target: { value: "https://example.com/a" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^备注/), {
      target: { value: "同事推荐" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
      url: "https://example.com/a",
      note: "同事推荐",
    });
  });

  it("服务端拒绝时在弹窗里显示原因，不关闭", async () => {
    serve();
    api.routes.set("POST /readlater", () => ({
      status: 400,
      body: { code: "invalid", message: "不能存本机或内网的地址" },
    }));
    renderIt("/readlater?new=1");
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("网址"), {
      target: { value: "http://127.0.0.1/x" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText("不能存本机或内网的地址"),
    ).toBeTruthy();
  });

  it("列表显示标题、摘要、标签、抓取状态和概要", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("鲸鱼协议详解")).toBeTruthy();
    expect(screen.getByText(/第一行讲协议/)).toBeTruthy();
    expect(screen.getByText("正在抓取")).toBeTruthy();
    expect(screen.getByText(/抓取失败 · 网页打不开，状态码 404/)).toBeTruthy();
    expect(screen.getAllByText("协议").length).toBeGreaterThan(0);
    // 默认看未读
    expect(api.calls.find((c) => c.method === "GET")?.path).toBe(
      "/readlater?view=unread",
    );
  });

  it("切到已读、点标签和搜索都会带进请求", async () => {
    serve();
    renderIt();
    await screen.findByText("鲸鱼协议详解");
    fireEvent.click(screen.getByRole("button", { name: "已读" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/readlater?view=read")).toBe(
        true,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: /^网络/ }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.path === "/readlater?view=read&tag=%E7%BD%91%E7%BB%9C",
        ),
      ).toBe(true),
    );
    fireEvent.change(screen.getByPlaceholderText("搜索存过的链接"), {
      target: { value: "心跳" },
    });
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.includes("q=%E5%BF%83%E8%B7%B3")),
      ).toBe(true),
    );
  });

  it("行上的按钮标为已读", async () => {
    serve();
    api.routes.set("PATCH /readlater/1", () => ({
      status: 200,
      body: { ...article, read: true },
    }));
    renderIt();
    await screen.findByText("鲸鱼协议详解");
    fireEvent.click(
      screen.getByRole("button", { name: "标为已读 鲸鱼协议详解" }),
    );
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        read: true,
      }),
    );
  });

  it("详情里能改标题和标签、重抓、写摘要、删除", async () => {
    serve();
    const detail = { ...article, content: "存下来的正文内容" };
    api.routes.set("GET /readlater/1", () => ({ status: 200, body: detail }));
    api.routes.set("PATCH /readlater/1", () => ({ status: 200, body: detail }));
    api.routes.set("POST /readlater/1/refetch", () => ({
      status: 200,
      body: detail,
    }));
    api.routes.set("POST /readlater/1/summarize", () => ({
      status: 200,
      body: detail,
    }));
    api.routes.set("DELETE /readlater/1", () => ({ status: 204 }));
    renderIt();
    fireEvent.click(await screen.findByText("鲸鱼协议详解"));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("存下来的正文内容")).toBeTruthy();
    expect(
      within(dialog)
        .getByRole("link", { name: /example\.com/ })
        .getAttribute("href"),
    ).toBe("https://example.com/whale");

    fireEvent.click(within(dialog).getByRole("button", { name: "AI 写摘要" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/readlater/1/summarize")).toBe(
        true,
      ),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "重新抓取" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/readlater/1/refetch")).toBe(
        true,
      ),
    );

    fireEvent.click(within(dialog).getByRole("button", { name: "编辑" }));
    fireEvent.change(within(dialog).getByLabelText("标题"), {
      target: { value: "我的标题" },
    });
    fireEvent.change(within(dialog).getByLabelText("标签，用逗号分开"), {
      target: { value: "#协议，同步、 Go" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        title: "我的标题",
        tags: ["协议", "同步", "Go"],
        note: "",
      }),
    );

    fireEvent.click(
      await within(dialog).findByRole("button", { name: /删除/ }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });
});

describe("SharePage B117", () => {
  it("详情里显示存档的图文，去掉脚本，有下载按钮；推文不完整时提示", async () => {
    serve();
    const detail = {
      ...article,
      kind: "tweet",
      hasHtml: true,
      meta: { incomplete: true },
      content: "推文文字",
      contentHtml:
        '<div class="xc-tweet"><p>推文文字</p><img alt="配图" src="/api/v1/readlater/1/assets/' +
        "a".repeat(64) +
        '" onerror="window.__hacked=1"><script>window.__hacked=1</script></div>',
    };
    api.routes.set("GET /readlater/1", () => ({ status: 200, body: detail }));
    renderIt();
    fireEvent.click(await screen.findByText("鲸鱼协议详解"));
    const dialog = await screen.findByRole("dialog");
    const img = await within(dialog).findByAltText("配图");
    expect(img.getAttribute("src")).toBe(
      "/api/v1/readlater/1/assets/" + "a".repeat(64),
    );
    expect(img.getAttribute("onerror")).toBeNull();
    expect(dialog.querySelector(".readlater-article script")).toBeNull();
    expect(within(dialog).getByText(/这条推文可能不完整/)).toBeTruthy();
    const download = within(dialog).getByRole("link", { name: /下载/ });
    expect(download.getAttribute("href")).toBe("/api/v1/readlater/1/export");
  });

  it("旧条目只有文字时提示重新抓取", async () => {
    serve();
    const detail = { ...article, content: "旧正文", hasHtml: false };
    api.routes.set("GET /readlater/1", () => ({ status: 200, body: detail }));
    renderIt();
    fireEvent.click(await screen.findByText("鲸鱼协议详解"));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/这一条只存了文字/)).toBeTruthy();
    expect(within(dialog).queryByRole("link", { name: /下载/ })).toBeNull();
  });

  it("从分享的文字里找出网址并保存", async () => {
    serve();
    api.routes.set("POST /readlater", () => ({
      status: 201,
      body: { item: article, duplicate: false },
    }));
    renderIt(
      "/readlater/share?title=%E6%96%87%E7%AB%A0&text=%E7%9C%8B%E8%BF%99%E4%B8%AA%20https%3A%2F%2Fexample.com%2Fwhale%EF%BC%8C%E4%B8%8D%E9%94%99",
    );
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        url: "https://example.com/whale",
        source: "share",
      }),
    );
    // 存好后回到稍后阅读
    expect(await screen.findByText("鲸鱼协议详解")).toBeTruthy();
    expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(1);
  });

  it("分享内容里没有网址时说明原因，不请求接口", async () => {
    renderIt("/readlater/share?text=%E6%B2%A1%E6%9C%89%E9%93%BE%E6%8E%A5");
    expect(await screen.findByText("分享的内容里没有找到网址。")).toBeTruthy();
    expect(api.calls.some((c) => c.method === "POST")).toBe(false);
  });

  it("保存失败时显示原因", async () => {
    api.routes.set("POST /readlater", () => ({
      status: 400,
      body: { code: "invalid", message: "不能存本机或内网的地址" },
    }));
    renderIt("/readlater/share?url=http%3A%2F%2Flocalhost%2Fx");
    expect(await screen.findByText("不能存本机或内网的地址")).toBeTruthy();
  });
});

describe("格式", () => {
  it("linkFromShare 先看 url 再看 text，去掉句尾标点", () => {
    expect(
      linkFromShare(
        new URLSearchParams("url=https://a.com/x&text=https://b.com"),
      ),
    ).toBe("https://a.com/x");
    expect(
      linkFromShare(new URLSearchParams({ text: "看 https://b.com/y?z=1)." })),
    ).toBe("https://b.com/y?z=1");
    expect(linkFromShare(new URLSearchParams({ text: "没有" }))).toBeNull();
  });
  it("parseTags 支持中英文逗号、顿号和井号", () => {
    expect(parseTags(" #a，b、c,\n d ,, ")).toEqual(["a", "b", "c", "d"]);
  });
});
