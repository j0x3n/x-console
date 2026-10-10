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
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import CredentialsPage from "./CredentialsPage";
import "./i18n";

function renderIt(url = "/credentials") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <CredentialsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  platform: "",
  account: "",
  usedBy: [] as string[],
  scopes: "",
  hint: "",
  hasSecret: false,
  createdOn: "",
  rotatedOn: "",
  expiresOn: "",
  rotateEveryDays: 0,
  remindDays: [30, 7],
  notes: "",
  archived: false,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};
const token = {
  ...base,
  id: 1,
  kind: "access_token",
  name: "部署令牌",
  hasSecret: true,
  platform: "GitHub",
  account: "me@example.com",
  usedBy: ["服务器 hk-1", "服务器 hk-2", "项目 x-console"],
  hint: "z9y8",
  createdOn: "2026-01-01",
  expiresOn: "2026-10-30",
  status: "soon",
  expiresIn: 20,
};
const old = {
  ...base,
  id: 2,
  kind: "api_key",
  name: "旧密钥",
  expiresOn: "2026-10-07",
  status: "expired",
  expiresIn: -3,
};
const stale = {
  ...base,
  id: 3,
  kind: "signing_key",
  name: "签名钥匙",
  createdOn: "2026-01-01",
  rotatedOn: "2026-05-01",
  rotateEveryDays: 90,
  status: "stale",
  rotateDueIn: -10,
};
const summary = { total: 3, expired: 1, soon: 1, stale: 1, none: 0 };

function serve(items = [token, old, stale], sum = summary) {
  api.routes.set("GET /credentials", () => ({
    status: 200,
    body: { items, summary: sum },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("CredentialsPage B120", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("密钥还没上线")).toBeTruthy();
  });

  it("没有记录时引导新建，保存时带上默认提醒天数", async () => {
    api.routes.set("GET /credentials", () => ({
      status: 200,
      body: {
        items: [],
        summary: { total: 0, expired: 0, soon: 0, stale: 0, none: 0 },
      },
    }));
    api.routes.set("POST /credentials", () => ({ status: 201, body: token }));
    renderIt();
    expect(await screen.findByText("还没有记录")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /新建记录/ })[0]!);
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText("只有“密钥内容”会加密存储，别的栏不要贴密钥。"),
    ).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("请填写名称")).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "部署令牌" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^用在/), {
      target: { value: "服务器 hk-1\n\n项目 x-console\n服务器 hk-1" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^到期日/), {
      target: { value: "2026-10-30" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toMatchObject({
      kind: "api_key",
      name: "部署令牌",
      usedBy: ["服务器 hk-1", "项目 x-console"],
      expiresOn: "2026-10-30",
      rotateEveryDays: 0,
      remindDays: [30, 7],
    });
  });

  it("更换周期要有起点，提醒天数写错时不提交", async () => {
    serve();
    renderIt("/credentials?new=1");
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "x" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^更换周期/), {
      target: { value: "90" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText(/设了更换周期，就要填创建日期/),
    ).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText(/^更换周期/), {
      target: { value: "" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^提前几天提醒/), {
      target: { value: "0, 30" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText(/提醒天数要写 1 到 3650/),
    ).toBeTruthy();
    expect(api.calls.some((c) => c.method === "POST")).toBe(false);
  });

  it("服务端拒绝像密钥的内容时把原因显示出来", async () => {
    serve();
    api.routes.set("POST /credentials", () => ({
      status: 400,
      body: {
        code: "invalid",
        message: "这看起来是密钥本身，请填到“密钥内容”里，其他栏不加密",
      },
    }));
    renderIt("/credentials?new=1");
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "ghp_x" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText(/这看起来是密钥本身/)).toBeTruthy();
  });

  it("列表显示名称、平台、用在哪里、状态和概要", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("部署令牌")).toBeTruthy();
    expect(screen.getByText("20 天后到期")).toBeTruthy();
    expect(screen.getByText("已过期 3 天")).toBeTruthy();
    expect(screen.getByText("超过更换周期 10 天")).toBeTruthy();
    expect(screen.getByText("GitHub · me@example.com")).toBeTruthy();
    expect(
      screen.getByText(/用在 服务器 hk-1、服务器 hk-2，共 3 处/),
    ).toBeTruthy();
    expect(screen.getByText("…z9y8")).toBeTruthy();
    const stat = (label: string) =>
      screen.getByText(label, { selector: ".xc-stat *" }).closest(".xc-stat")!
        .textContent;
    expect(stat("久未更换")).toContain("1");
  });

  it("点概要卡片按状态筛选，类型下拉按类型筛选", async () => {
    serve();
    renderIt();
    await screen.findByText("部署令牌");
    fireEvent.click(
      screen
        .getByText("久未更换", { selector: ".xc-stat *" })
        .closest("button")!,
    );
    expect(screen.queryByText("部署令牌")).toBeNull();
    expect(screen.getByText("签名钥匙")).toBeTruthy();
    fireEvent.click(
      screen
        .getByText("久未更换", { selector: ".xc-stat *" })
        .closest("button")!,
    );
    fireEvent.change(screen.getByLabelText("类型"), {
      target: { value: "api_key" },
    });
    expect(screen.getByText("旧密钥")).toBeTruthy();
    expect(screen.queryByText("部署令牌")).toBeNull();
  });

  it("搜索能按用在哪里找，详情里点一项会按它搜索", async () => {
    serve();
    renderIt();
    await screen.findByText("部署令牌");
    fireEvent.change(screen.getByPlaceholderText("搜索记录"), {
      target: { value: "HK-2" },
    });
    expect(screen.queryByText("旧密钥")).toBeNull();
    fireEvent.change(screen.getByPlaceholderText("搜索记录"), {
      target: { value: "" },
    });
    fireEvent.click(screen.getByText("部署令牌"));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "项目 x-console" }),
    );
    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText("搜索记录") as HTMLInputElement).value,
      ).toBe("项目 x-console"),
    );
    expect(screen.queryByText("旧密钥")).toBeNull();
    expect(screen.getByText("部署令牌")).toBeTruthy();
  });

  it("存了密钥内容的记录：默认不显示，点显示才取出来，保存时可以带上新的内容", async () => {
    serve();
    api.routes.set("GET /credentials/1/secret", () => ({
      status: 200,
      body: { secret: "ghp_secret_value" },
    }));
    api.routes.set("PATCH /credentials/1", () => ({
      status: 200,
      body: token,
    }));
    renderIt();
    fireEvent.click(await screen.findByText("部署令牌"));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("已加密保存")).toBeTruthy();
    expect(within(dialog).queryByText("ghp_secret_value")).toBeNull();
    fireEvent.click(within(dialog).getByRole("button", { name: /显示/ }));
    expect(await within(dialog).findByText("ghp_secret_value")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: /隐藏/ }));
    expect(within(dialog).queryByText("ghp_secret_value")).toBeNull();

    // 修改时不填内容就不带 secret，勾选清除才带空字符串
    fireEvent.click(within(dialog).getByRole("button", { name: /编辑/ }));
    const edit = (await screen.findAllByRole("dialog")).at(-1)!;
    fireEvent.click(within(edit).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    expect(
      api.calls.find((c) => c.method === "PATCH")?.body as object,
    ).not.toHaveProperty("secret");
  });

  it("显示已归档会带上 archived 参数", async () => {
    serve();
    renderIt();
    await screen.findByText("部署令牌");
    fireEvent.click(screen.getByLabelText("显示已归档"));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/credentials?archived=true"),
      ).toBe(true),
    );
  });

  it("详情里点已更换，带上更换日期和沿用有效期推出的新到期日", async () => {
    serve();
    api.routes.set("POST /credentials/1/rotate", () => ({
      status: 200,
      body: token,
    }));
    renderIt();
    fireEvent.click(await screen.findByText("部署令牌"));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("…z9y8")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: /已更换/ }));
    const expiry = within(dialog).getByLabelText(
      "新的到期日",
    ) as HTMLInputElement;
    // 创建于 2026-01-01，到期 2026-10-30，有效期 302 天，从今天往后推
    expect(expiry.value).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    fireEvent.change(expiry, { target: { value: "2027-01-01" } });
    fireEvent.change(within(dialog).getByLabelText("识别尾号"), {
      target: { value: "n3w4" },
    });
    fireEvent.click(
      within(dialog).getAllByRole("button", { name: "保存" })[0]!,
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/credentials/1/rotate")).toBe(
        true,
      ),
    );
    expect(
      api.calls.find((c) => c.path === "/credentials/1/rotate")?.body,
    ).toMatchObject({ expiresOn: "2027-01-01", hint: "n3w4" });
  });

  it("归档和删除", async () => {
    serve();
    api.routes.set("PATCH /credentials/1", () => ({
      status: 200,
      body: token,
    }));
    api.routes.set("DELETE /credentials/1", () => ({ status: 204 }));
    renderIt();
    fireEvent.click(await screen.findByText("部署令牌"));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "归档" }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
      archived: true,
    });
    cleanup();
    renderIt();
    fireEvent.click(await screen.findByText("部署令牌"));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "删除" }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/credentials/1",
        ),
      ).toBe(true),
    );
  });
});
