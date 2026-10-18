// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 假的 WebSocket：记下开了几个，测试里手动触发 onopen。
const sockets = vi.hoisted(() => {
  class FakeSocket {
    static OPEN = 1;
    static instances: FakeSocket[] = [];
    url: string;
    readyState = 0;
    closed = false;
    binaryType = "";
    onopen: (() => void) | null = null;
    onmessage: ((e: { data: unknown }) => void) | null = null;
    onclose: ((e: { reason: string }) => void) | null = null;
    constructor(url: string) {
      this.url = url;
      FakeSocket.instances.push(this);
    }
    open() {
      this.readyState = 1;
      this.onopen?.();
    }
    send() {}
    close() {
      this.closed = true;
      this.readyState = 3;
    }
  }
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket;
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  return FakeSocket;
});

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    loadAddon() {}
    open(el: HTMLElement) {
      const node = document.createElement("div");
      node.className = "xterm";
      el.appendChild(node);
    }
    write() {}
    focus() {}
    dispose() {}
    onData() {}
    onBinary() {}
    onResize() {}
    onTitleChange() {}
  },
}));
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit() {}
  },
}));
vi.mock("../components/TerminalScripts", () => ({ default: () => null }));
vi.mock("../../../auth/elevation", () => ({
  withElevation: async () => undefined,
}));

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useParams } from "react-router";
import type { HostDetail } from "../api";
import TerminalTab from "../components/TerminalTab";
import {
  BACKGROUND_TTL_MS,
  backgroundSessions,
  disconnect,
  expireSessions,
  useTerminals,
} from "./store";
import { usePreferencesStore } from "../../../stores/preferences-store";
import { useToastStore } from "../../../hooks/useToast";
import "../i18n";

const hostOf = (id: string) =>
  ({
    id,
    name: `机器${id}`,
    kind: "server",
    online: true,
    capabilities: ["pty"],
  }) as unknown as HostDetail;

function HostPage() {
  const { hostId = "" } = useParams();
  return <TerminalTab key={hostId} host={hostOf(hostId)} />;
}

function setup(start = "/servers/h1/terminal") {
  const router = createMemoryRouter(
    [
      { path: "/servers/:hostId/terminal", element: <HostPage /> },
      { path: "/notes", element: <p>笔记页</p> },
    ],
    { initialEntries: [start] },
  );
  render(<RouterProvider router={router} />);
  return router;
}

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)));

async function connect() {
  fireEvent.click(screen.getByRole("button", { name: /连接/ }));
  await flush();
  const ws = sockets.instances.at(-1)!;
  act(() => ws.open());
  expect(screen.getByText("已连接")).toBeTruthy();
  return ws;
}

async function go(router: ReturnType<typeof setup>, to: string) {
  await act(() => router.navigate(to));
  await flush();
}

beforeEach(() => {
  sockets.instances = [];
  usePreferencesStore.setState({ terminalLeave: "ask" });
});

afterEach(async () => {
  cleanup();
  await flush();
  for (const id of Object.keys(useTerminals.getState().sessions))
    disconnect(id);
});

describe("background terminals (B249)", () => {
  it("asks before leaving and keeps the same connection", async () => {
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    expect(screen.getByText("终端还连着")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "留在后台" }));
    await flush();
    expect(screen.getByText("笔记页")).toBeTruthy();
    expect(ws.closed).toBe(false);
    expect(
      backgroundSessions(useTerminals.getState().sessions).map((s) => s.hostId),
    ).toEqual(["h1"]);

    await go(router, "/servers/h1/terminal");
    expect(screen.getByText("已连接")).toBeTruthy();
    expect(sockets.instances).toHaveLength(1);
    expect(useTerminals.getState().sessions.h1.parkedAt).toBeNull();
  });

  it("disconnects when told to", async () => {
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "断开" }),
    );
    await flush();
    expect(screen.getByText("笔记页")).toBeTruthy();
    expect(ws.closed).toBe(true);
    expect(useTerminals.getState().sessions).toEqual({});
  });

  it("stays on the page when the dialog is dismissed", async () => {
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    fireEvent.keyDown(document, { key: "Escape" });
    await flush();
    expect(router.state.location.pathname).toBe("/servers/h1/terminal");
    expect(ws.closed).toBe(false);
  });

  it("remembers the choice and stops asking", async () => {
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "留在后台" }));
    await flush();
    expect(usePreferencesStore.getState().terminalLeave).toBe("keep");

    await go(router, "/servers/h1/terminal");
    await go(router, "/notes");
    expect(screen.queryByText("终端还连着")).toBeNull();
    expect(screen.getByText("笔记页")).toBeTruthy();
    expect(ws.closed).toBe(false);
  });

  it("disconnects without asking when the choice is close", async () => {
    usePreferencesStore.setState({ terminalLeave: "close" });
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    expect(screen.queryByText("终端还连着")).toBeNull();
    expect(ws.closed).toBe(true);
  });

  it("does not ask when the terminal is not connected", async () => {
    const router = setup();
    await go(router, "/notes");
    expect(screen.queryByText("终端还连着")).toBeNull();
    expect(screen.getByText("笔记页")).toBeTruthy();
  });

  it("disconnects terminals idle in the background for 30 minutes", async () => {
    usePreferencesStore.setState({ terminalLeave: "keep" });
    const router = setup();
    const ws = await connect();
    await go(router, "/notes");
    const parkedAt = useTerminals.getState().sessions.h1.parkedAt!;
    expect(expireSessions(parkedAt + BACKGROUND_TTL_MS - 1000)).toEqual([]);
    expect(ws.closed).toBe(false);
    const gone = expireSessions(parkedAt + BACKGROUND_TTL_MS);
    expect(gone.map((s) => s.hostId)).toEqual(["h1"]);
    expect(ws.closed).toBe(true);
    expect(useTerminals.getState().sessions).toEqual({});
  });

  it("keeps at most 4 and drops the oldest", async () => {
    usePreferencesStore.setState({ terminalLeave: "keep" });
    const router = setup("/notes");
    const opened = [];
    for (const id of ["h1", "h2", "h3", "h4", "h5"]) {
      await go(router, `/servers/${id}/terminal`);
      opened.push(await connect());
      await go(router, "/notes");
    }
    expect(opened[0].closed).toBe(true);
    expect(opened.slice(1).every((ws) => !ws.closed)).toBe(true);
    expect(
      backgroundSessions(useTerminals.getState().sessions).map((s) => s.hostId),
    ).toEqual(["h2", "h3", "h4", "h5"]);
    expect(useToastStore.getState().current?.message).toBe(
      "已断开 机器h1 的终端，后台最多留 4 个",
    );
  });
});
