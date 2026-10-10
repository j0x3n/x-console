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
import { MemoryRouter } from "react-router";
import ContactsPage from "./ContactsPage";
import "./i18n";

function renderIt(url = "/contacts") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <ContactsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  events: [] as unknown[],
  lastContactOn: "",
  contactEveryDays: 0,
  remindDays: [7, 1],
  notes: "",
  phones: [] as string[],
  emails: [] as string[],
  source: "",
  archived: false,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};
const wang = {
  ...base,
  id: 1,
  name: "老王",
  group: "friend",
  events: [
    {
      id: "e1",
      kind: "birthday",
      label: "生日",
      date: "1990-10-15",
      nextOn: "2026-10-15",
      nextIn: 5,
      years: 36,
    },
  ],
  lastContactOn: "2026-09-20",
  status: "soon",
  nextEventIn: 5,
  nextEventLabel: "生日",
  sinceContact: 20,
};
const li = {
  ...base,
  id: 2,
  name: "小李",
  group: "colleague",
  lastContactOn: "2026-06-01",
  contactEveryDays: 60,
  status: "overdue",
  sinceContact: 131,
  contactDueIn: -71,
};
const zhao = { ...base, id: 3, name: "赵姐", group: "family", status: "ok" };
const summary = { total: 3, soon: 1, overdue: 1 };

function serve(items = [wang, li, zhao], sum = summary) {
  api.routes.set("GET /contacts", () => ({
    status: 200,
    body: { items, summary: sum },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("ContactsPage B122", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("联系人还没上线")).toBeTruthy();
  });

  it("没有联系人时引导新建，重要日期和提醒天数一起保存", async () => {
    api.routes.set("GET /contacts", () => ({
      status: 200,
      body: { items: [], summary: { total: 0, soon: 0, overdue: 0 } },
    }));
    api.routes.set("POST /contacts", () => ({ status: 201, body: wang }));
    renderIt();
    expect(await screen.findByText("还没有联系人")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /新建联系人/ })[0]!);
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("请填写名称")).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "老王" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: /添加日期/ }));
    fireEvent.change(within(dialog).getByLabelText("重要日期 1 日期"), {
      target: { value: "1990-02-30" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText(/日期要写成/)).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("重要日期 1 日期"), {
      target: { value: "1990-10-15" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toMatchObject({
      name: "老王",
      group: "other",
      events: [{ kind: "birthday", date: "1990-10-15" }],
      remindDays: [7, 1],
      contactEveryDays: 0,
    });
  });

  it("不能填两个生日，提醒天数写错时不提交", async () => {
    serve();
    renderIt("/contacts?new=1");
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("名称"), {
      target: { value: "x" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: /添加日期/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: /添加日期/ }));
    fireEvent.change(within(dialog).getByLabelText("重要日期 2 类型"), {
      target: { value: "birthday" },
    });
    fireEvent.change(within(dialog).getByLabelText("重要日期 1 类型"), {
      target: { value: "birthday" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText("一个联系人只能有一个生日"),
    ).toBeTruthy();
    fireEvent.click(
      within(dialog).getAllByRole("button", { name: /^移除/ })[0]!,
    );
    fireEvent.change(within(dialog).getByLabelText("重要日期 1 日期"), {
      target: { value: "03-08" },
    });
    fireEvent.change(within(dialog).getByLabelText(/^提前几天提醒/), {
      target: { value: "0, 7" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText(/提醒天数要写 1 到 365/),
    ).toBeTruthy();
    expect(api.calls.some((c) => c.method === "POST")).toBe(false);
  });

  it("列表显示名称、上次联系、状态和概要", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("老王")).toBeTruthy();
    expect(screen.getByText("生日 5 天后")).toBeTruthy();
    expect(screen.getByText("该联系了 · 71 天")).toBeTruthy();
    expect(screen.getByText("上次联系 20 天前")).toBeTruthy();
    expect(screen.getByText("上次联系 还没记过")).toBeTruthy();
    const stat = (label: string) =>
      screen.getByText(label, { selector: ".xc-stat *" }).closest(".xc-stat")!
        .textContent;
    expect(stat("该联系了")).toContain("1");
    expect(stat("近期的日子")).toContain("1");
  });

  it("点概要卡片按状态筛选，分组下拉按分组筛选，搜索按名字找", async () => {
    serve();
    renderIt();
    await screen.findByText("老王");
    fireEvent.click(
      screen
        .getByText("该联系了", { selector: ".xc-stat *" })
        .closest("button")!,
    );
    expect(screen.queryByText("老王")).toBeNull();
    expect(screen.getByText("小李")).toBeTruthy();
    fireEvent.click(
      screen
        .getByText("该联系了", { selector: ".xc-stat *" })
        .closest("button")!,
    );
    fireEvent.change(screen.getByLabelText("分组"), {
      target: { value: "family" },
    });
    expect(screen.getByText("赵姐")).toBeTruthy();
    expect(screen.queryByText("老王")).toBeNull();
    fireEvent.change(screen.getByLabelText("分组"), { target: { value: "" } });
    fireEvent.change(screen.getByPlaceholderText("搜索联系人"), {
      target: { value: "小" },
    });
    expect(screen.queryByText("老王")).toBeNull();
    expect(screen.getByText("小李")).toBeTruthy();
  });

  it("列表上点刚联系过会记一次联系，不打开详情", async () => {
    serve();
    api.routes.set("POST /contacts/2/touch", () => ({
      status: 200,
      body: li,
    }));
    renderIt();
    await screen.findByText("小李");
    const row = screen.getByText("小李").closest(".contacts-row")!;
    fireEvent.click(
      within(row as HTMLElement).getByRole("button", { name: /刚联系过/ }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/contacts/2/touch")).toBe(true),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("详情显示每个日期的下一次，能归档和删除", async () => {
    serve();
    api.routes.set("PATCH /contacts/1", () => ({ status: 200, body: wang }));
    api.routes.set("DELETE /contacts/1", () => ({ status: 204 }));
    renderIt();
    fireEvent.click(await screen.findByText("老王"));
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText(/下一次 2026-10-15 · 5 天后 · 36 岁/),
    ).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "归档" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
      archived: true,
    });
    cleanup();
    renderIt();
    fireEvent.click(await screen.findByText("老王"));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "删除" }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/contacts/1",
        ),
      ).toBe(true),
    );
  });

  it("显示已归档会带上 archived 参数", async () => {
    serve();
    renderIt();
    await screen.findByText("老王");
    fireEvent.click(screen.getByLabelText("显示已归档"));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/contacts?archived=true")).toBe(
        true,
      ),
    );
  });

  it("导入和同步：没设置时填 Apple ID 和应用专用密码连接", async () => {
    serve();
    api.routes.set("GET /contacts/sync", () => ({
      status: 200,
      body: {
        configured: false,
        syncing: false,
        total: 0,
        created: 0,
        updated: 0,
      },
    }));
    api.routes.set("PUT /contacts/sync", () => ({
      status: 200,
      body: {
        configured: true,
        username: "me@icloud.com",
        syncing: false,
        total: 3,
        created: 3,
        updated: 0,
      },
    }));
    renderIt();
    await screen.findByText("老王");
    fireEvent.click(screen.getByRole("button", { name: /导入和同步/ }));
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByTestId("vcf-input").getAttribute("accept"),
    ).toContain(".vcf");
    const connect = within(dialog).getByRole("button", { name: "连接并同步" });
    expect((connect as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("Apple ID"), {
      target: { value: "me@icloud.com" },
    });
    fireEvent.change(within(dialog).getByLabelText("应用专用密码"), {
      target: { value: "abcd-efgh-ijkl-mnop" },
    });
    fireEvent.click(connect);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      username: "me@icloud.com",
      password: "abcd-efgh-ijkl-mnop",
      onlyWithDates: false,
    });
  });

  it("导入和同步：已连接时显示账号、上次结果和失败原因", async () => {
    serve();
    api.routes.set("GET /contacts/sync", () => ({
      status: 200,
      body: {
        configured: true,
        username: "me@icloud.com",
        syncing: false,
        lastSyncAt: "2026-10-10T01:00:00Z",
        lastError: "登录失败：Apple ID 或应用专用密码不对",
        total: 12,
        created: 2,
        updated: 1,
      },
    }));
    api.routes.set("POST /contacts/sync/run", () => ({
      status: 200,
      body: {
        configured: true,
        username: "me@icloud.com",
        syncing: false,
        total: 12,
        created: 0,
        updated: 0,
      },
    }));
    renderIt();
    await screen.findByText("老王");
    fireEvent.click(screen.getByRole("button", { name: /导入和同步/ }));
    const dialog = screen.getByRole("dialog");
    expect(await within(dialog).findByText("me@icloud.com")).toBeTruthy();
    expect(within(dialog).getByText(/上次同步失败/)).toBeTruthy();
    expect(within(dialog).getByText(/12 人，新增 2，更新 1/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: /立即同步/ }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/contacts/sync/run")).toBe(true),
    );
  });

  it("联系人详情显示电话和邮箱", async () => {
    serve([
      {
        ...wang,
        phones: ["138 0000 0000"],
        emails: ["wang@example.com"],
        source: "icloud",
      },
    ]);
    renderIt();
    fireEvent.click(await screen.findByText("老王"));
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog)
        .getByText("138 0000 0000")
        .closest("a")
        ?.getAttribute("href"),
    ).toBe("tel:13800000000");
    expect(within(dialog).getByText("wang@example.com")).toBeTruthy();
    expect(within(dialog).getByText("来自 iCloud 同步")).toBeTruthy();
  });
});
