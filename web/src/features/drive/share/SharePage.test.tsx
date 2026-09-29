// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 页面之前替换。
const api = vi.hoisted(() => {
  const calls: Array<{ method: string; path: string; body: string }> = [];
  const state = { unlocked: false, live: true };
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
  const json = (status: number, body: unknown) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input, init);
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    calls.push({
      method: req.method,
      path: path + url.search,
      body: await req.text(),
    });
    if (!state.live)
      return json(501, { code: "not_live", message: "还没上线" });
    if (path === "/public/shares/abc/unlock")
      return (JSON.parse(calls[calls.length - 1].body) as { code: string })
        .code === "k2m9"
        ? ((state.unlocked = true),
          json(200, { access: "tok", expiresAt: "2026-09-30T00:00:00Z" }))
        : json(403, {
            code: "share_code_wrong",
            message: "提取码不对，还能试 4 次",
          });
    if (path === "/public/shares/abc")
      return url.searchParams.get("t") === "tok"
        ? json(200, {
            name: "报告.pdf",
            isDir: false,
            size: 2048,
            mime: "application/pdf",
          })
        : json(401, { code: "share_code_required", message: "要提取码" });
    return json(404, { code: "share_not_found", message: "没有" });
  }) as typeof fetch;
  return { calls, state };
});

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import SharePage, { publicUrl } from "./SharePage";

function renderShare(token = "abc") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <SharePage token={token} />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  api.calls.length = 0;
  api.state.live = true;
  sessionStorage.clear();
});

describe("SharePage", () => {
  it("asks for the access code, then shows the file", async () => {
    renderShare();
    const input = await screen.findByLabelText("提取码");
    fireEvent.change(input, { target: { value: "zzzz" } });
    fireEvent.click(screen.getByRole("button", { name: "打开" }));
    expect(await screen.findByText("提取码不对，还能试 4 次")).toBeTruthy();
    fireEvent.change(input, { target: { value: "k2m9" } });
    fireEvent.click(screen.getByRole("button", { name: "打开" }));
    expect(await screen.findByText("报告.pdf")).toBeTruthy();
    const link = screen.getByRole("link", { name: /下载/ });
    expect(link.getAttribute("href")).toBe(
      "/api/v1/public/shares/abc/content?t=tok",
    );
    expect(sessionStorage.getItem("xc.share.abc")).toBe("tok");
  });

  it("says a dead link does not work", async () => {
    renderShare("gone");
    expect(await screen.findByText("链接打不开")).toBeTruthy();
  });

  it("says sharing is not live when the server has no share API", async () => {
    api.state.live = false;
    renderShare();
    expect(await screen.findByText("分享还没上线")).toBeTruthy();
  });

  it("builds public file addresses", () => {
    expect(publicUrl("a b", "zip")).toBe("/api/v1/public/shares/a%20b/zip");
    expect(publicUrl("abc", "content", { item: 3, inline: true })).toBe(
      "/api/v1/public/shares/abc/content?item=3&inline=1",
    );
  });
});
