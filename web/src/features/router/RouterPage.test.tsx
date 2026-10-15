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
import RouterPage from "./RouterPage";
import RouterSettingsTab from "./RouterSettingsTab";
import "./i18n";

function renderIt(node: React.ReactNode, path = "/") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const status = {
  source: "ubus",
  hostname: "OpenWrt",
  model: "Xiaomi AX3600",
  firmware: "OpenWrt 24.10.0",
  uptimeSeconds: 90000,
  interfaces: [
    {
      name: "lan",
      up: true,
      ipv4: ["192.168.1.1/24"],
      ipv6: [],
      uptimeSeconds: 90000,
    },
    {
      name: "wan",
      up: true,
      proto: "pppoe",
      ipv4: ["100.64.1.2/32"],
      ipv6: [],
      uptimeSeconds: 3600,
    },
  ],
  wan: {
    name: "wan",
    up: true,
    proto: "pppoe",
    ipv4: ["100.64.1.2/32"],
    ipv6: [],
    uptimeSeconds: 3600,
  },
  rxRate: 2 * 1024 * 1024,
  txRate: 300 * 1024,
  clientCount: 2,
  checkedAt: "2026-10-01T12:00:00Z",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  localStorage.clear();
});

describe("RouterPage B65", () => {
  it("没配置时引导去设置", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 412,
      body: { code: "integration_not_configured", message: "集成还没有配置" },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("连接 OpenWrt 路由器")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "设置路由器" }).getAttribute("href"),
    ).toBe("/settings/router");
  });

  it("显示概要、接口、在线设备，重启接口调接口", async () => {
    api.routes.set("GET /router/status", () => ({ status: 200, body: status }));
    api.routes.set("GET /router/clients", () => ({
      status: 200,
      body: {
        items: [
          { mac: "AA:BB:CC:00:00:03", name: "phone", ip: "192.168.1.3" },
          { mac: "AA:BB:CC:00:00:02", name: "nas", ip: "192.168.1.20" },
        ],
      },
    }));
    api.routes.set("POST /router/interfaces/wan/restart", () => ({
      status: 204,
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("在线")).toBeTruthy();
    expect(screen.getByText("100.64.1.2")).toBeTruthy();
    expect(screen.getByText("↓ 2.0 MB/s")).toBeTruthy();
    expect(await screen.findByText("phone")).toBeTruthy();
    expect(screen.getByText("AA:BB:CC:00:00:02")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "重启接口 wan" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/router/interfaces/wan/restart"),
      ).toBe(true),
    );
  });

  it("B114：路由器主动上报时重启接口是排队", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: { ...status, source: "push" },
    }));
    api.routes.set("GET /router/clients", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /router/interfaces/wan/restart", () => ({
      status: 202,
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("100.64.1.2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重启接口 wan" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/router/interfaces/wan/restart"),
      ).toBe(true),
    );
  });

  it("B114：很久没收到上报时页面显示原因", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 502,
      body: {
        code: "router_unreachable",
        message: "已经 5 分钟没收到路由器的上报，上一次在 12:00",
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText(/没收到路由器的上报/)).toBeTruthy();
  });

  it("流量标签按范围取数据", async () => {
    localStorage.setItem("router.tab", "traffic");
    api.routes.set("GET /router/status", () => ({ status: 200, body: status }));
    api.routes.set("GET /router/traffic", () => ({
      status: 200,
      body: {
        range: "7d",
        stepSeconds: 3600,
        points: [],
        rxBytes: 0,
        txBytes: 0,
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("还没有流量数据")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "最近 7 天" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/router/traffic?range=7d")).toBe(
        true,
      ),
    );
  });
});

describe("RouterSettingsTab B65", () => {
  it("保存时带上表单，密码留空不传", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: {
        url: "http://192.168.1.1",
        username: "xconsole",
        mode: "direct",
        hasPassword: true,
      },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("PUT /router/config", (body) => ({
      status: 200,
      body: { ...(body as object), hasPassword: true },
    }));
    renderIt(<RouterSettingsTab />);
    expect(await screen.findByDisplayValue("http://192.168.1.1")).toBeTruthy();
    expect(screen.getByPlaceholderText("留空表示不改")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "保存并检查" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const put = api.calls.find((c) => c.method === "PUT");
    expect(put?.body).toEqual({
      url: "http://192.168.1.1",
      username: "xconsole",
      mode: "direct",
    });
  });
});

describe("RouterSettingsTab B114", () => {
  it("选“路由器主动上报”后生成令牌，显示安装命令", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: { url: "", username: "", mode: "direct", hasPassword: false },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("POST /router/push/token", () => ({
      status: 200,
      body: {
        token: "t0k3n",
        reportUrl: "https://x.example/api/v1/router/report",
        script:
          "cat > /usr/bin/xc-report.sh <<'XC_EOF'\nTOKEN='t0k3n'\nXC_EOF\n",
        installUrl: "https://x.example/api/v1/router/install/abc",
        command:
          "wget -qO /tmp/xc-install.sh 'https://x.example/api/v1/router/install/abc' && sh /tmp/xc-install.sh",
        installExpiresAt: "2026-10-15T08:15:00Z",
      },
    }));
    renderIt(<RouterSettingsTab />);
    const select = await screen.findByLabelText("连接方式");
    fireEvent.change(select, { target: { value: "push" } });
    // 地址、用户名、密码和保存按钮都不显示
    expect(screen.queryByPlaceholderText("http://192.168.1.1")).toBeNull();
    expect(screen.queryByRole("button", { name: "保存并检查" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "生成上报令牌" }));
    // B227：先显示一行命令，完整脚本收在“手动安装”里
    expect(
      await screen.findByText(/wget -qO \/tmp\/xc-install.sh/),
    ).toBeTruthy();
    expect(screen.getByText("手动安装")).toBeTruthy();
    expect(screen.getByText(/TOKEN='t0k3n'/)).toBeTruthy();
    expect(screen.getByText(/ca-bundle/)).toBeTruthy();
    expect(api.calls.some((c) => c.path === "/router/push/token")).toBe(true);
  });

  it("改上报间隔会保存", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: {
        url: "",
        username: "",
        mode: "push",
        hasPassword: false,
        pushInterval: 60,
      },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("PUT /router/push/interval", (body) => ({
      status: 200,
      body: {
        url: "",
        username: "",
        mode: "push",
        hasPassword: false,
        pushInterval: (body as { seconds: number }).seconds,
      },
    }));
    renderIt(<RouterSettingsTab />);
    const select = await screen.findByLabelText("上报间隔");
    fireEvent.change(select, { target: { value: "3" } });
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.path === "/router/push/interval" && c.method === "PUT",
        ),
      ).toBe(true),
    );
    expect(
      api.calls.find((c) => c.path === "/router/push/interval")?.body,
    ).toEqual({ seconds: 3 });
  });

  it("已经在主动上报时显示最近一次上报，重新生成要确认", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: {
        url: "",
        username: "",
        mode: "push",
        hasPassword: false,
        reportUrl: "https://x.example/api/v1/router/report",
      },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    renderIt(<RouterSettingsTab />);
    expect(await screen.findByText(/还没有收到过上报/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "重新生成令牌" })).toBeTruthy();
  });
});

const pushStatus = { ...status, source: "push" };
const phone = {
  mac: "AA:BB:CC:00:00:03",
  name: "iPhone",
  note: "我的手机",
  displayName: "我的手机",
  ip: "192.168.1.3",
  conn: "wifi",
  wifiIface: "wlan1",
  online: true,
  onlineSince: "2026-10-01T11:00:00Z",
  lastSeen: "2026-10-01T12:00:00Z",
  firstSeen: "2026-09-01T12:00:00Z",
  notify: false,
  cooldownMinutes: null,
};
const tv = {
  mac: "AA:BB:CC:00:00:09",
  name: "tv",
  note: "",
  displayName: "tv",
  ip: "192.168.1.9",
  conn: "wired",
  online: false,
  lastSeen: "2026-10-01T10:00:00Z",
  firstSeen: "2026-09-01T12:00:00Z",
  notify: false,
  cooldownMinutes: null,
};
const options = {
  deviceNotify: true,
  newDeviceNotify: true,
  notifyCooldownMinutes: 15,
  logDays: 7,
};

describe("RouterPage B228 设备", () => {
  function setup() {
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    api.routes.set("GET /router/devices", () => ({
      status: 200,
      body: { items: [phone, tv], trafficAvailable: false },
    }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
  }

  it("默认只看在线的，切到全部能看到离线设备，可以搜", async () => {
    setup();
    renderIt(<RouterPage />);
    expect(await screen.findByText("我的手机")).toBeTruthy();
    expect(screen.queryByText("tv")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "全部" }));
    expect(await screen.findByText("tv")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("搜名字、备注、IP 或 MAC"), {
      target: { value: "00:09" },
    });
    expect(screen.queryByText("我的手机")).toBeNull();
    expect(screen.getByText("tv")).toBeTruthy();
  });

  it("点设备打开详情，改备注和通知开关", async () => {
    setup();
    api.routes.set("PATCH /router/devices/AABBCC000003", (body) => ({
      status: 200,
      body: { ...phone, ...(body as object) },
    }));
    renderIt(<RouterPage />);
    fireEvent.click(await screen.findByText("我的手机"));
    const dialog = await screen.findByRole("dialog", { name: "我的手机" });
    expect(dialog.textContent).toContain("AA:BB:CC:00:00:03");
    fireEvent.change(screen.getByDisplayValue("我的手机"), {
      target: { value: "老婆的手机" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        note: "老婆的手机",
        notify: false,
        cooldownMinutes: null,
      }),
    );
    fireEvent.click(screen.getByLabelText("上下线通知"));
    await waitFor(() =>
      expect(
        api.calls.filter((c) => c.method === "PATCH").at(-1)?.body,
      ).toMatchObject({ notify: true }),
    );
  });

  it("忘掉设备调删除接口", async () => {
    setup();
    api.routes.set("DELETE /router/devices/AABBCC000003", () => ({
      status: 204,
    }));
    renderIt(<RouterPage />);
    fireEvent.click(await screen.findByText("我的手机"));
    fireEvent.click(
      await screen.findByRole("button", { name: "忘掉这台设备" }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) =>
            c.method === "DELETE" && c.path === "/router/devices/AABBCC000003",
        ),
      ).toBe(true),
    );
  });
});

describe("RouterSettingsTab B228 设备通知", () => {
  it("主动上报模式下能开关总开关、改默认间隔", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: { url: "", username: "", mode: "push", hasPassword: false },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: { ...options, deviceNotify: false },
    }));
    api.routes.set("PUT /router/options", (body) => ({
      status: 200,
      body,
    }));
    renderIt(<RouterSettingsTab />);
    fireEvent.click(await screen.findByLabelText("设备上下线时通知"));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        ...options,
        deviceNotify: true,
      }),
    );
    fireEvent.change(screen.getByLabelText(/默认再次通知间隔/), {
      target: { value: "60" },
    });
    await waitFor(() =>
      expect(
        api.calls.filter((c) => c.method === "PUT").at(-1)?.body,
      ).toMatchObject({ notifyCooldownMinutes: 60 }),
    );
  });
});

describe("B232 新设备通知", () => {
  it("通知链接带 device 时直接打开这台设备，流量页签也一样", async () => {
    localStorage.setItem("router.tab", "traffic");
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    api.routes.set("GET /router/devices", () => ({
      status: 200,
      body: { items: [phone, tv], trafficAvailable: false },
    }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
    renderIt(<RouterPage />, "/router?device=AA:BB:CC:00:00:09");
    // 离线的设备也能打开
    expect(await screen.findByRole("dialog", { name: "tv" })).toBeTruthy();
  });

  it("设置里能关掉新设备通知", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: { url: "", username: "", mode: "push", hasPassword: false },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
    api.routes.set("PUT /router/options", (body) => ({ status: 200, body }));
    renderIt(<RouterSettingsTab />);
    fireEvent.click(await screen.findByLabelText("新设备接入时通知"));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        ...options,
        newDeviceNotify: false,
      }),
    );
  });
});

describe("B229 设备流量", () => {
  it("有 nlbwmon 时显示速率和今天的流量，详情里取曲线", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    api.routes.set("GET /router/devices", () => ({
      status: 200,
      body: {
        items: [
          {
            ...phone,
            rxRate: 3 * 1024 * 1024,
            txRate: 0,
            todayRx: 2 * 1024 * 1024 * 1024,
            todayTx: 0,
          },
        ],
        trafficAvailable: true,
      },
    }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
    api.routes.set("GET /router/devices/AABBCC000003/traffic", () => ({
      status: 200,
      body: {
        range: "24h",
        stepSeconds: 3600,
        points: [],
        rxBytes: 0,
        txBytes: 0,
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("3.0 MB/s")).toBeTruthy();
    expect(screen.getByText("2.0 GB")).toBeTruthy();
    expect(screen.queryByText(/装 nlbwmon/)).toBeNull();
    fireEvent.click(screen.getByText("我的手机"));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.path === "/router/devices/AABBCC000003/traffic?range=24h",
        ),
      ).toBe(true),
    );
  });

  it("没有 nlbwmon 时提示安装", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    api.routes.set("GET /router/devices", () => ({
      status: 200,
      body: { items: [phone], trafficAvailable: false },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText(/装 nlbwmon/)).toBeTruthy();
    expect(screen.queryByText("↓")).toBeNull();
  });

  it("设备合计比 WAN 少很多时提示", async () => {
    localStorage.setItem("router.tab", "traffic");
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    const GB = 1024 * 1024 * 1024;
    api.routes.set("GET /router/traffic", () => ({
      status: 200,
      body: {
        range: "24h",
        stepSeconds: 60,
        points: [],
        rxBytes: 0,
        txBytes: 0,
        devicesTodayRx: GB / 10,
        devicesTodayTx: 0,
        wanTodayRx: 2 * GB,
        wanTodayTx: 0,
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText(/设备流量比 WAN 少很多/)).toBeTruthy();
  });
});

describe("B230 日志", () => {
  function setup() {
    localStorage.setItem("router.tab", "logs");
    api.routes.set("GET /router/status", () => ({
      status: 200,
      body: pushStatus,
    }));
    api.routes.set("GET /router/devices", () => ({
      status: 200,
      body: { items: [phone, tv], trafficAvailable: false },
    }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
    api.routes.set("GET /router/events", () => ({
      status: 200,
      body: {
        items: [
          {
            id: 9,
            at: "2026-10-01T11:59:00Z",
            kind: "device_offline",
            mac: phone.mac,
            title: "我的手机 离线了",
            detail: "",
          },
          {
            id: 8,
            at: "2026-10-01T11:00:00Z",
            kind: "wan_down",
            title: "WAN 口掉线了",
            detail: "掉线 3 分钟",
          },
        ],
        hasMore: true,
      },
    }));
    api.routes.set("GET /router/logs", () => ({
      status: 200,
      body: {
        items: [
          {
            id: 3,
            at: "2026-10-01T11:59:00Z",
            level: "err",
            facility: "daemon",
            program: "dnsmasq",
            message: "failed to bind",
          },
        ],
        hasMore: false,
        programs: ["dnsmasq", "netifd"],
      },
    }));
  }

  it("事件按类型筛选、能翻页，点设备事件打开设备", async () => {
    setup();
    renderIt(<RouterPage />);
    expect(await screen.findByText("WAN 口掉线了")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("事件类型"), {
      target: { value: "wan" },
    });
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.startsWith("/router/events?group=wan")),
      ).toBe(true),
    );
    fireEvent.click(await screen.findByRole("button", { name: "加载更多" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.includes("before=8"))).toBe(true),
    );
    fireEvent.click(screen.getAllByText("我的手机 离线了")[0]);
    expect(
      await screen.findByRole("dialog", { name: "我的手机" }),
    ).toBeTruthy();
  });

  it("系统日志按级别和程序筛选", async () => {
    setup();
    localStorage.setItem("router.logs.part", "system");
    renderIt(<RouterPage />);
    expect(await screen.findByText("failed to bind")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("级别"), {
      target: { value: "err" },
    });
    fireEvent.change(screen.getByLabelText("程序"), {
      target: { value: "dnsmasq" },
    });
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) =>
            c.path.startsWith("/router/logs") &&
            c.path.includes("level=err") &&
            c.path.includes("program=dnsmasq"),
        ),
      ).toBe(true),
    );
  });

  it("直连模式没有日志页签", async () => {
    localStorage.setItem("router.tab", "logs");
    api.routes.set("GET /router/status", () => ({ status: 200, body: status }));
    api.routes.set("GET /router/traffic", () => ({
      status: 200,
      body: {
        range: "24h",
        stepSeconds: 60,
        points: [],
        rxBytes: 0,
        txBytes: 0,
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("100.64.1.2")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "日志" })).toBeNull();
  });
});

describe("RouterSettingsTab B230 日志保存天数", () => {
  it("改保存天数", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: { url: "", username: "", mode: "push", hasPassword: false },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("GET /router/options", () => ({
      status: 200,
      body: options,
    }));
    api.routes.set("PUT /router/options", (body) => ({ status: 200, body }));
    renderIt(<RouterSettingsTab />);
    fireEvent.change(await screen.findByLabelText(/保存天数/), {
      target: { value: "15" },
    });
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        ...options,
        logDays: 15,
      }),
    );
  });
});
