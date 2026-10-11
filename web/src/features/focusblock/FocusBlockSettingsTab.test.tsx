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
    calls.push({ method: req.method, path, body });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(body)
      : { status: 404, body: { code: "not_found", message: "not found" } };
    return new Response(
      reply.body === undefined ? null : JSON.stringify(reply.body),
      {
        status: reply.status,
        headers: { "Content-Type": "application/json" },
      },
    );
  }) as typeof fetch;
  return { routes, calls };
});

const toasts = vi.hoisted(() => [] as unknown[]);
vi.mock("../../hooks/useToast", () => ({
  toast: (t: unknown) => toasts.push(t),
}));

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
}));
vi.mock("../../auth/elevation", () => ({
  withElevation: <T,>(fn: () => Promise<T>) => fn(),
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
import FocusBlockBadge from "./FocusBlockBadge";
import FocusBlockSettingsTab from "./FocusBlockSettingsTab";
import "./i18n";

const computer = (extra: object = {}) => ({
  id: "a1",
  name: "办公室电脑",
  os: "windows",
  online: true,
  supported: true,
  selected: true,
  state: "active",
  message: "已生效。已屏蔽 6 个网址名，已结束 2 个程序",
  sites: 6,
  killed: 2,
  refused: [],
  updatedAt: "2026-10-12T09:00:00Z",
  ...extra,
});

const settings = (extra: object = {}) => ({
  enabled: true,
  hostIds: ["a1"],
  domains: ["youtube.com"],
  apps: ["steam"],
  ipv6: false,
  blockInBreak: false,
  haStart: "",
  haEnd: "",
  haMessage: "",
  computers: [computer()],
  maxDomains: 200,
  maxApps: 100,
  ...extra,
});

function renderIt(ui = <FocusBlockSettingsTab />) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const toastText = () =>
  toasts.map((t) =>
    typeof t === "string" ? t : (t as { message: string }).message,
  );

function serve(initial: unknown) {
  toasts.length = 0;
  api.routes.clear();
  api.calls.length = 0;
  let current = initial as Record<string, unknown>;
  api.routes.set("GET /focus-block/settings", () => ({
    status: 200,
    body: current,
  }));
  api.routes.set("PUT /focus-block/settings", (body) => {
    current = { ...current, ...(body as object) };
    return { status: 200, body: current };
  });
}

const puts = () => api.calls.filter((c) => c.method === "PUT");

afterEach(cleanup);

describe("专注屏蔽设置", () => {
  it("显示总开关、电脑状态和名单", async () => {
    serve(settings());
    renderIt();
    expect(await screen.findByText("办公室电脑")).toBeTruthy();
    expect(screen.getByText("屏蔽已生效")).toBeTruthy();
    expect(
      screen.getByText("已生效。已屏蔽 6 个网址名，已结束 2 个程序"),
    ).toBeTruthy();
    expect(
      (screen.getByLabelText("要屏蔽的网站") as HTMLTextAreaElement).value,
    ).toBe("youtube.com");
    expect(
      (screen.getByLabelText("要结束的程序") as HTMLTextAreaElement).value,
    ).toBe("steam");
    // 使用前要知道：安全 DNS 和手机
    expect(screen.getByText(/安全 DNS/)).toBeTruthy();
    expect(screen.getByText("只管电脑，管不到手机和平板。")).toBeTruthy();
  });

  it("权限不足和离线都有明确的状态", async () => {
    serve(
      settings({
        computers: [
          computer({
            state: "permission",
            message: "没有改 hosts 文件的权限，网站没有屏蔽。",
          }),
          computer({
            id: "b2",
            name: "笔记本",
            online: false,
            state: "offline",
            message: "不在线",
          }),
          computer({
            id: "c3",
            name: "旧服务器",
            supported: false,
            selected: false,
            state: "unsupported",
            message: "代理版本太旧，更新代理后才能用",
          }),
        ],
      }),
    );
    renderIt();
    expect(await screen.findByText("权限不足")).toBeTruthy();
    expect(screen.getByText("离线")).toBeTruthy();
    expect(screen.getByText("要更新代理")).toBeTruthy();
    // 版本太旧的电脑不能选
    const boxes = screen.getAllByRole("checkbox") as HTMLInputElement[];
    const old = boxes.find((b) =>
      b.closest("li")?.textContent?.includes("旧服务器"),
    );
    expect(old?.disabled).toBe(true);
  });

  it("开关和电脑勾选马上保存", async () => {
    serve(settings({ enabled: false }));
    renderIt();
    fireEvent.click(await screen.findByLabelText("开启专注屏蔽"));
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0].body).toEqual({ enabled: true });
    fireEvent.click(screen.getByRole("checkbox", { name: /办公室电脑/ }));
    await waitFor(() => expect(puts()).toHaveLength(2));
    expect(puts()[1].body).toEqual({ hostIds: [] });
    fireEvent.click(screen.getByLabelText("休息时也屏蔽"));
    await waitFor(() => expect(puts()).toHaveLength(3));
    expect(puts()[2].body).toEqual({ blockInBreak: true });
  });

  it("名单改过才能保存，保存时整理成数组", async () => {
    serve(settings());
    renderIt();
    const save = (await screen.findByText("保存名单")).closest(
      "button",
    ) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("要屏蔽的网站"), {
      target: { value: "youtube.com\nhttps://www.bilibili.com/\n\n" },
    });
    fireEvent.change(screen.getByLabelText("要结束的程序"), {
      target: { value: "steam\nSteam.exe\nvlc" },
    });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0].body).toEqual({
      domains: ["youtube.com", "https://www.bilibili.com/"],
      apps: ["steam", "vlc"],
    });
  });

  it("服务端拒绝名单时显示原因，输入保持原样", async () => {
    serve(settings());
    api.routes.set("PUT /focus-block/settings", () => ({
      status: 400,
      body: {
        code: "validation_failed",
        message: "“explorer”是系统关键程序，不能结束",
      },
    }));
    renderIt();
    fireEvent.change(await screen.findByLabelText("要结束的程序"), {
      target: { value: "explorer" },
    });
    fireEvent.click(screen.getByText("保存名单"));
    await waitFor(() =>
      expect(toastText().join()).toContain("系统关键程序，不能结束"),
    );
    expect(
      (screen.getByLabelText("要结束的程序") as HTMLTextAreaElement).value,
    ).toBe("explorer");
  });

  it("一键导入电脑时间里的娱乐类程序，只加没有的", async () => {
    serve(settings());
    api.routes.set("GET /focus-block/suggestions", () => ({
      status: 200,
      body: { apps: ["steam", "spotify", "cloudmusic"] },
    }));
    renderIt();
    fireEvent.click(await screen.findByText("导入娱乐类程序"));
    await waitFor(() =>
      expect(
        (screen.getByLabelText("要结束的程序") as HTMLTextAreaElement).value,
      ).toBe("steam\nspotify\ncloudmusic"),
    );
    // 导入只改输入框，点“保存名单”才保存
    expect(puts()).toHaveLength(0);
    expect(toastText()).toContain("2 个程序已加进名单。点“保存名单”后生效。");
  });

  it("没有配对的电脑时给出去设备页的入口", async () => {
    serve(settings({ computers: [], hostIds: [] }));
    renderIt();
    expect(await screen.findByText(/还没有配对的电脑/)).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "设备与代理" }).getAttribute("href"),
    ).toBe("/settings/devices");
  });

  it("Home Assistant 的场景可以留空，填了才保存", async () => {
    serve(settings());
    renderIt();
    const save = (await screen.findByText("保存 Home Assistant 设置")).closest(
      "button",
    ) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("开始时的场景或脚本"), {
      target: { value: " scene.focus " },
    });
    fireEvent.click(save);
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0].body).toEqual({ haStart: "scene.focus", haEnd: "" });
  });

  it("接口还没上线时显示“还没上线”", async () => {
    api.routes.clear();
    renderIt();
    expect(await screen.findByText(/还没上线|还没有上线/)).toBeTruthy();
  });
});

describe("番茄钟上的屏蔽小标", () => {
  const status = (body: object) => {
    api.routes.clear();
    api.routes.set("GET /focus-block/status", () => ({ status: 200, body }));
  };

  it("屏蔽生效时显示“屏蔽中”", async () => {
    status({ blocking: true, activeComputers: 1, problemComputers: 0 });
    renderIt(<FocusBlockBadge />);
    expect(await screen.findByText("屏蔽中")).toBeTruthy();
  });

  it("选了电脑但没成功时显示“屏蔽有问题”，点进设置页", async () => {
    status({ blocking: false, activeComputers: 0, problemComputers: 1 });
    renderIt(<FocusBlockBadge />);
    const link = await screen.findByRole("link", { name: /屏蔽有问题/ });
    expect(link.getAttribute("href")).toBe("/settings/focus-block");
  });

  it("没在屏蔽、接口还没上线时什么都不显示", async () => {
    status({ blocking: false, activeComputers: 0, problemComputers: 0 });
    const { container } = renderIt(<FocusBlockBadge />);
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/focus-block/status")).toBe(
        true,
      ),
    );
    expect(container.textContent).toBe("");
    cleanup();
    api.routes.clear();
    const gone = renderIt(<FocusBlockBadge />);
    await new Promise((r) => setTimeout(r, 30));
    expect(gone.container.textContent).toBe("");
  });
});
