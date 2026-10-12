// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Outlet, RouterProvider, createMemoryRouter } from "react-router";
import SettingsHost from "./SettingsHost";
import { routes as settingsRoutes } from "./routes";
import { settingsTarget } from "./nav";

// 设置页的接口在这里都回 404，只看弹窗怎么开、怎么关、怎么换页。
globalThis.fetch = (async () =>
  new Response(JSON.stringify({ code: "not_found", message: "not found" }), {
    status: 404,
    headers: { "Content-Type": "application/json" },
  })) as typeof fetch;

function Shell() {
  return (
    <>
      <Outlet />
      <SettingsHost />
    </>
  );
}

function show(path: string) {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: <Shell />,
        children: [
          { index: true, element: <p>today</p> },
          { path: "notes", element: <p>notes page</p> },
          { path: "cards", element: <p>cards page</p> },
          ...settingsRoutes,
        ],
      },
    ],
    { initialEntries: [path] },
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

const where = (router: ReturnType<typeof show>) =>
  router.state.location.pathname + router.state.location.search;

afterEach(cleanup);

describe("settings dialog (B180)", () => {
  // 地址记的是上一个页面，这一条要排在最前面
  it("opening a settings address directly uses the today page", async () => {
    const router = show("/settings/storage?gdrive=ok");
    await screen.findByRole("dialog", { name: "设置" });
    expect(where(router)).toBe("/?gdrive=ok&settings=storage");
    fireEvent.click(screen.getAllByRole("button", { name: "关闭设置" })[0]);
    await waitFor(() => expect(where(router)).toBe("/?gdrive=ok"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens old /settings links on top of the current page", async () => {
    const router = show("/notes?id=5");
    await screen.findByText("notes page");
    await act(() => router.navigate("/settings/linear"));
    // Linear 并进了 Git 那一页
    expect(where(router)).toBe("/notes?id=5&settings=git");
    expect(await screen.findByRole("dialog", { name: "设置" })).toBeTruthy();
    expect(screen.getByText("notes page")).toBeTruthy();
    expect(
      screen.getByRole("button", { current: "page" }).textContent,
    ).toContain("Git");
  });

  it("switches tabs without new history and closes with Esc", async () => {
    const router = show("/notes");
    await screen.findByText("notes page");
    await act(() => router.navigate("/settings/security"));
    await screen.findByRole("dialog", { name: "设置" });
    fireEvent.click(screen.getByRole("button", { name: /备份/ }));
    expect(where(router)).toBe("/notes?settings=backup");
    fireEvent.keyDown(document, { key: "Escape" });
    await screen.findByText("notes page");
    expect(where(router)).toBe("/notes");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("sends settings that moved out to their own pages", async () => {
    const router = show("/settings/cards");
    await screen.findByText("cards page");
    expect(where(router)).toBe("/cards?tab=settings");
  });
});

describe("settingsTarget", () => {
  it("turns ?section into a hash", () => {
    expect(
      settingsTarget(
        "assistant",
        new URL("http://x/settings/assistant?section=ai-memory"),
      ),
    ).toMatch(/settings=assistant#ai-memory$/);
  });
});
