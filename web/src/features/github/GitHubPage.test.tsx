// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 页面之前替换。
const responses = vi.hoisted(() => {
  const routes = new Map<string, unknown>();
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
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    return new Response(JSON.stringify(routes.get(path) ?? []), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return { routes };
});

import "./i18n";
import GitHubPage from "./GitHubPage";

function renderPage(path = "/github") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <GitHubPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  author: "jo", baseRef: "main", draft: false, reviewState: "none", issueKeys: [],
  createdAt: "2026-09-20T10:00:00Z", updatedAt: "2026-09-25T10:00:00Z",
};

afterEach(() => {
  cleanup();
  responses.routes.clear();
});

describe("GitHubPage", () => {
  it("asks to connect when there is no token", async () => {
    responses.routes.set("/github/status", { configured: false, syncing: false, repoCount: 0 });
    renderPage();
    const link = await screen.findByRole("link", { name: /去设置/ });
    expect(link.getAttribute("href")).toBe("/settings/github");
  });

  it("groups pull requests by repo with check badges and issue links", async () => {
    responses.routes.set("/github/status", { configured: true, syncing: false, repoCount: 2, lastSyncAt: "2026-09-25T10:00:00Z" });
    responses.routes.set("/github/pulls", [
      { ...base, repo: "acme/web", number: 3, title: "Fix login", url: "https://github.com/acme/web/pull/3", headRef: "xc-12", checkState: "failure", issueKeys: ["XC-12"] },
      { ...base, repo: "acme/api", number: 8, title: "Add cache", url: "https://github.com/acme/api/pull/8", headRef: "xc/5-cache", checkState: "success", reviewState: "approved", codingTaskId: 5 },
    ]);
    renderPage();
    const headings = await screen.findAllByRole("heading", { level: 2 });
    expect(headings.map((h) => h.textContent)).toEqual(["acme/api", "acme/web"]);
    expect(screen.getByText("检查失败")).toBeTruthy();
    expect(screen.getByText("已批准")).toBeTruthy();
    expect(screen.getByRole("link", { name: "XC-12" }).getAttribute("href")).toBe("/projects/XC/12");
    expect(screen.getByRole("link", { name: /编码任务 #5/ }).getAttribute("href")).toBe("/coding/5");

    responses.routes.set("/github/runs", [
      { id: 1, repo: "acme/api", name: "CI", branch: "main", event: "push", status: "completed", conclusion: "failure", url: "https://x", defaultBranch: true, createdAt: "2026-09-25T10:00:00Z", updatedAt: "2026-09-25T10:00:00Z" },
    ]);
    fireEvent.click(screen.getByRole("button", { name: "CI 运行" }));
    expect(await screen.findByText("默认分支")).toBeTruthy();
    expect(screen.getAllByText("失败").length).toBeGreaterThan(0);
  });
});
