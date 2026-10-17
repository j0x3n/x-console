// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const server = vi.hoisted(() => {
  const state = {
    note: {
      title: "旅行清单",
      body: "带护照",
      updatedAt: "2026-10-17T01:00:00Z",
      canEdit: true,
    },
    conflict: false,
    puts: [] as Record<string, unknown>[],
  };
  const NativeRequest = globalThis.Request;
  globalThis.Request = class extends NativeRequest {
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
    const req =
      input instanceof NativeRequest ? input : new Request(String(input), init);
    if (req.method === "PUT") {
      const body = (await req.json()) as Record<string, unknown>;
      state.puts.push(body);
      if (state.conflict) {
        state.note = {
          ...state.note,
          body: "别人改过",
          updatedAt: "2026-10-17T02:00:00Z",
        };
        return json(409, { code: "note_changed", message: "这篇笔记刚被改过" });
      }
      state.note = {
        ...state.note,
        title: String(body.title),
        body: String(body.body),
        updatedAt: "2026-10-17T03:00:00Z",
      };
    }
    return json(200, state.note);
  }) as typeof fetch;
  return state;
});

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import NoteSharePage from "./NoteSharePage";

function open() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <NoteSharePage token="tok" />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  server.note = {
    title: "旅行清单",
    body: "带护照",
    updatedAt: "2026-10-17T01:00:00Z",
    canEdit: true,
  };
  server.conflict = false;
  server.puts.length = 0;
});

describe("B243 分享页编辑", () => {
  it("没开编辑时没有编辑按钮", async () => {
    server.note.canEdit = false;
    open();
    await screen.findByText("带护照");
    expect(screen.queryByRole("button", { name: /编辑/ })).toBeNull();
  });

  it("改了正文点保存，带上开始编辑时的修改时间和称呼", async () => {
    open();
    fireEvent.click(await screen.findByRole("button", { name: /编辑/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "笔记" }), {
      target: { value: "带护照和充电器" },
    });
    fireEvent.change(
      screen.getByRole("textbox", { name: "你的称呼（可以不填）" }),
      {
        target: { value: "小王" },
      },
    );
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByText("带护照和充电器");
    expect(server.puts[0]).toEqual({
      title: "旅行清单",
      body: "带护照和充电器",
      baseUpdatedAt: "2026-10-17T01:00:00Z",
      editorName: "小王",
    });
  });

  it("别人改过时提示，载入最新内容后自己写的放在下面", async () => {
    server.conflict = true;
    open();
    fireEvent.click(await screen.findByRole("button", { name: /编辑/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "笔记" }), {
      target: { value: "我的版本" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByText("这篇笔记刚被改过");
    fireEvent.click(screen.getByRole("button", { name: "载入最新内容" }));
    await waitFor(() =>
      expect(
        (screen.getByRole("textbox", { name: "笔记" }) as HTMLTextAreaElement)
          .value,
      ).toBe("别人改过"),
    );
    const mine = screen.getByRole("textbox", {
      name: "你刚才写的内容，需要的部分复制过去",
    }) as HTMLTextAreaElement;
    expect(mine.value).toBe("我的版本");
    expect(screen.queryByText("这篇笔记刚被改过")).toBeNull();
  });
});
