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
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path, body });
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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DigestCard, { timesError } from "./DigestCard";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

const ids = [
  "alert",
  "security",
  "agent_decision",
  "reminder",
  "brief",
  "agent_progress",
  "mail",
  "github",
  "project",
  "other",
];
const nowIds = ["alert", "security", "agent_decision", "reminder", "brief"];

function status(over: Record<string, unknown> = {}) {
  return {
    enabled: true,
    times: ["11:00", "15:00", "19:00"],
    categories: ids.map((id) => {
      const mode = nowIds.includes(id) ? "now" : "digest";
      return { id, mode, defaultMode: mode };
    }),
    pending: 3,
    oldestAt: new Date(Date.now() - 3600_000).toISOString(),
    ...over,
  };
}

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DigestCard />
    </QueryClientProvider>,
  );
}

describe("timesError", () => {
  it("accepts filled, unique HH:MM values", () => {
    expect(timesError(["11:00", "15:00"])).toBe(false);
  });
  it("rejects empty, malformed and repeated values", () => {
    expect(timesError([])).toBe(true);
    expect(timesError(["11:00", ""])).toBe(true);
    expect(timesError(["25:00"])).toBe(true);
    expect(timesError(["11:00", "11:00"])).toBe(true);
  });
});

describe("DigestCard", () => {
  it("shows the waiting count and the three send times", async () => {
    api.routes.set("GET /notify/digest", () => ({
      status: 200,
      body: status(),
    }));
    renderCard();
    expect(
      await screen.findByText("条通知等待发送", { exact: false }),
    ).toBeTruthy();
    expect(screen.getByText("3").tagName).toBe("STRONG");
    expect(
      (screen.getByLabelText("发送时间 1") as HTMLInputElement).value,
    ).toBe("11:00");
    expect(
      (screen.getByLabelText("发送时间 3") as HTMLInputElement).value,
    ).toBe("19:00");
    expect((screen.getByLabelText("新邮件") as HTMLSelectElement).value).toBe(
      "digest",
    );
    expect(
      (screen.getByLabelText("提醒和习惯") as HTMLSelectElement).value,
    ).toBe("now");
  });

  it("sends the changed type and time when saved", async () => {
    api.routes.set("GET /notify/digest", () => ({
      status: 200,
      body: status(),
    }));
    let saved: unknown;
    api.routes.set("PUT /notify/digest", (body) => {
      saved = body;
      return { status: 200, body: status() };
    });
    renderCard();
    const save = (await screen.findByRole("button", {
      name: "保存",
    })) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("服务器和监控告警"), {
      target: { value: "digest" },
    });
    fireEvent.change(screen.getByLabelText("发送时间 2"), {
      target: { value: "16:30" },
    });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(saved).toBeDefined());
    const body = saved as {
      enabled: boolean;
      times: string[];
      modes: Record<string, string>;
    };
    expect(body.enabled).toBe(true);
    expect(body.times).toEqual(["11:00", "16:30", "19:00"]);
    expect(body.modes.alert).toBe("digest");
    expect(body.modes.mail).toBe("digest");
    expect(body.modes.reminder).toBe("now");
  });

  it("blocks saving a duplicated time", async () => {
    api.routes.set("GET /notify/digest", () => ({
      status: 200,
      body: status(),
    }));
    renderCard();
    const save = (await screen.findByRole("button", {
      name: "保存",
    })) as HTMLButtonElement;
    fireEvent.change(screen.getByLabelText("发送时间 2"), {
      target: { value: "11:00" },
    });
    expect(save.disabled).toBe(true);
  });

  it("sends the waiting notifications now", async () => {
    api.routes.set("GET /notify/digest", () => ({
      status: 200,
      body: status(),
    }));
    api.routes.set("POST /notify/digest/flush", () => ({
      status: 200,
      body: { count: 3, delivered: 1 },
    }));
    renderCard();
    fireEvent.click(
      await screen.findByRole("button", { name: /现在发送积压/ }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "POST" && c.path === "/notify/digest/flush",
        ),
      ).toBe(true),
    );
  });

  it("disables the send button when nothing is waiting", async () => {
    api.routes.set("GET /notify/digest", () => ({
      status: 200,
      body: status({ pending: 0, oldestAt: undefined }),
    }));
    renderCard();
    const button = (await screen.findByRole("button", {
      name: /现在发送积压/,
    })) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });
});
