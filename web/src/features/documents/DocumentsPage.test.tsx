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
import DocumentsPage from "./DocumentsPage";
import "./i18n";

function renderIt(url = "/documents") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <DocumentsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  holder: "",
  number: "",
  issuedOn: "",
  expiresOn: "",
  currency: "",
  serial: "",
  remindDays: [90, 30, 7],
  notes: "",
  files: [],
  archived: false,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};
const passport = {
  ...base,
  id: 1,
  kind: "passport",
  name: "李四的护照",
  holder: "李四",
  number: "E12345678",
  expiresOn: "2026-10-30",
  status: "soon",
  daysLeft: 20,
  files: [{ driveId: 7, name: "护照.jpg" }],
};
const insurance = {
  ...base,
  id: 2,
  kind: "insurance",
  name: "旧车险",
  expiresOn: "2026-10-07",
  status: "expired",
  daysLeft: -3,
};
const contract = {
  ...base,
  id: 3,
  kind: "contract",
  name: "租房合同",
  status: "none",
};
const summary = { total: 3, expired: 1, soon: 1, none: 1 };

function serve(items = [passport, insurance, contract], sum = summary) {
  api.routes.set("GET /documents", () => ({
    status: 200,
    body: { items, summary: sum },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("DocumentsPage B115", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("证件档案还没上线")).toBeTruthy();
  });

  it("没有档案时引导新建，保存时带上默认提醒天数", async () => {
    api.routes.set("GET /documents", () => ({
      status: 200,
      body: {
        items: [],
        summary: { total: 0, expired: 0, soon: 0, none: 0 },
      },
    }));
    api.routes.set("POST /documents", () => ({ status: 201, body: passport }));
    renderIt();
    expect(await screen.findByText("还没有档案")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /新建档案/ })[0]!);
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("请填写名称")).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "李四的护照" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^到期日/), {
      target: { value: "2026-10-30" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toMatchObject({
      kind: "passport",
      name: "李四的护照",
      expiresOn: "2026-10-30",
      remindDays: [90, 30, 7],
    });
  });

  it("提醒天数写错时不提交", async () => {
    serve();
    renderIt("/documents?new=1");
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "x" },
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

  it("列表显示名称、持有人、剩余天数和概要", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("李四的护照")).toBeTruthy();
    expect(screen.getByText("20 天后到期")).toBeTruthy();
    expect(screen.getByText("已过期 3 天")).toBeTruthy();
    expect(
      screen.getByText("没有到期日", { selector: ".xc-badge" }),
    ).toBeTruthy();
    expect(screen.getByText(/李四/, { selector: "small" })).toBeTruthy();
  });

  it("二级菜单选的视图和类型决定列表", async () => {
    serve();
    renderIt("/documents?view=expired");
    expect(await screen.findByText("旧车险")).toBeTruthy();
    expect(screen.queryByText("李四的护照")).toBeNull();
    cleanup();
    renderIt("/documents?kind=contract");
    expect(await screen.findByText("租房合同")).toBeTruthy();
    expect(screen.queryByText("旧车险")).toBeNull();
  });

  it("搜索只留下符合的", async () => {
    serve();
    renderIt();
    await screen.findByText("李四的护照");
    fireEvent.change(screen.getByPlaceholderText("搜索档案"), {
      target: { value: "租房" },
    });
    expect(screen.queryByText("李四的护照")).toBeNull();
    expect(screen.getByText("租房合同")).toBeTruthy();
  });

  it("显示已归档会带上 archived 参数", async () => {
    serve();
    renderIt();
    await screen.findByText("李四的护照");
    fireEvent.click(screen.getByLabelText("显示已归档"));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/documents?archived=true")).toBe(
        true,
      ),
    );
  });

  it("详情里续期只改到期日", async () => {
    serve();
    api.routes.set("PATCH /documents/1", () => ({
      status: 200,
      body: passport,
    }));
    renderIt();
    fireEvent.click(await screen.findByText("李四的护照"));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("E12345678")).toBeTruthy();
    expect(within(dialog).getByText("护照.jpg")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "续期" }));
    fireEvent.change(within(dialog).getByLabelText("新的到期日"), {
      target: { value: "2036-10-30" },
    });
    fireEvent.click(
      within(dialog).getAllByRole("button", { name: "保存" })[0]!,
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
      expiresOn: "2036-10-30",
    });
  });

  it("归档、去掉文件、删除", async () => {
    serve();
    api.routes.set("PATCH /documents/1", () => ({
      status: 200,
      body: passport,
    }));
    api.routes.set("DELETE /documents/1/files/7", () => ({
      status: 200,
      body: { ...passport, files: [] },
    }));
    api.routes.set("DELETE /documents/1", () => ({ status: 204 }));
    renderIt();
    fireEvent.click(await screen.findByText("李四的护照"));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: /从档案里去掉 护照.jpg/ }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/documents/1/files/7")).toBe(
        true,
      ),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "归档" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
      archived: true,
    });
    cleanup();
    renderIt();
    fireEvent.click(await screen.findByText("李四的护照"));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "删除" }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/documents/1",
        ),
      ).toBe(true),
    );
  });
});
