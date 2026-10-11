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

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
}));

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import CardFromSelection from "./SelectionCard";
import "./i18n";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  window.getSelection()?.removeAllRanges();
});

const TEXT = "WAL 模式把写入先追加到日志文件，读操作仍然读主数据库文件。";
const source = { title: "SQLite 笔记", link: "/notes/3" };

function settings(aiAvailable: boolean, aiEnabled = true) {
  api.routes.set("GET /cards/settings", () => ({
    status: 200,
    body: {
      newPerDay: 10,
      reviewPerDay: 100,
      retention: 0.9,
      pushEnabled: true,
      pushTime: "20:00",
      aiEnabled,
      aiAvailable,
    },
  }));
  api.routes.set("GET /cards/decks", () => ({ status: 200, body: [] }));
}

function renderIt(disabled = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <div data-testid="scope">
          <p data-testid="text">{TEXT}</p>
          <textarea data-testid="box" defaultValue={TEXT} />
          <CardFromSelection source={source} disabled={disabled} />
        </div>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function selectParagraph(
  el: HTMLElement,
  from = 0,
  to = el.textContent!.length,
) {
  const text = el.firstChild!;
  const range = document.createRange();
  range.setStart(text, from);
  range.setEnd(text, to);
  const sel = window.getSelection()!;
  sel.removeAllRanges();
  sel.addRange(range);
  fireEvent.mouseUp(el, { clientX: 100, clientY: 100 });
}

describe("make a card from a selection", () => {
  it("offers the button for selected text and saves a hand-written card", async () => {
    settings(false);
    api.routes.set("POST /cards", () => ({ status: 201, body: { id: 5 } }));
    renderIt();
    expect(screen.queryByText("做成卡片")).toBeNull();

    selectParagraph(screen.getByTestId("text"));
    fireEvent.click(await screen.findByText("做成卡片"));

    // Without AI the selected text is the draft answer and the question is blank.
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog.textContent).toContain("AI 还没配置"));
    const [question, answer] = Array.from(
      dialog.querySelectorAll<HTMLTextAreaElement>("textarea"),
    );
    expect(answer.value).toBe(TEXT);
    expect(screen.getByText("保存").closest("button")!.disabled).toBe(true);
    fireEvent.change(question, {
      target: { value: "WAL 模式下读写为什么不互相阻塞？" },
    });
    fireEvent.click(screen.getByText("保存"));

    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        question: "WAL 模式下读写为什么不互相阻塞？",
        answer: TEXT,
        sourceTitle: "SQLite 笔记",
        sourceLink: "/notes/3",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(api.calls.some((c) => c.path === "/cards/generate")).toBe(false);
  });

  it("asks the AI for a draft when it is set up, and lets the user change it", async () => {
    settings(true);
    api.routes.set("POST /cards/generate", () => ({
      status: 200,
      body: { question: "WAL 把写入先追加到哪里？", answer: "日志文件" },
    }));
    api.routes.set("POST /cards", () => ({ status: 201, body: { id: 6 } }));
    renderIt();
    selectParagraph(screen.getByTestId("text"));
    fireEvent.click(await screen.findByText("做成卡片"));

    const dialog = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(dialog.textContent).toContain("会发给你的 AI 供应商"),
    );
    await waitFor(() => {
      const [q] = Array.from(
        dialog.querySelectorAll<HTMLTextAreaElement>("textarea"),
      );
      expect(q.value).toBe("WAL 把写入先追加到哪里？");
    });
    expect(api.calls.find((c) => c.path === "/cards/generate")?.body).toEqual({
      text: TEXT,
      style: "qa",
      sourceTitle: "SQLite 笔记",
    });

    // Switching the style asks again.
    api.routes.set("POST /cards/generate", () => ({
      status: 200,
      body: { question: "WAL 把写入先追加到 ____。", answer: "日志文件" },
    }));
    fireEvent.click(screen.getByText("填空"));
    await waitFor(() => {
      const [q] = Array.from(
        dialog.querySelectorAll<HTMLTextAreaElement>("textarea"),
      );
      expect(q.value).toContain("____");
    });
    expect(
      api.calls.filter((c) => c.path === "/cards/generate").pop()?.body,
    ).toMatchObject({
      style: "cloze",
    });
    fireEvent.click(screen.getByText("保存"));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.method === "POST" && c.path === "/cards")?.body,
      ).toMatchObject({
        question: "WAL 把写入先追加到 ____。",
        answer: "日志文件",
      }),
    );
  });

  it("does not call the AI when the user turned it off", async () => {
    settings(true, false);
    renderIt();
    selectParagraph(screen.getByTestId("text"));
    fireEvent.click(await screen.findByText("做成卡片"));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog.textContent).toContain("关掉了 AI"));
    expect(api.calls.some((c) => c.path === "/cards/generate")).toBe(false);
    expect(screen.queryByText("用 AI 写")).toBeNull();
  });

  it("hides words in a fill-in-the-blank card by hand", async () => {
    settings(false);
    renderIt();
    selectParagraph(screen.getByTestId("text"));
    fireEvent.click(await screen.findByText("做成卡片"));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(screen.getByText("填空"));
    const [question, answer] = Array.from(
      dialog.querySelectorAll<HTMLTextAreaElement>("textarea"),
    );
    expect(question.value).toBe(TEXT);
    expect(answer.value).toBe("");
    question.setSelectionRange(10, 14);
    fireEvent.click(screen.getByText("把选中的字挖空"));
    expect(question.value).toBe(TEXT.slice(0, 10) + "____" + TEXT.slice(14));
    expect(answer.value).toBe(TEXT.slice(10, 14));
  });

  it("reads a selection inside an edit box", async () => {
    settings(false);
    renderIt();
    const box = screen.getByTestId("box") as HTMLTextAreaElement;
    box.focus();
    box.setSelectionRange(0, 20);
    fireEvent.select(box);
    fireEvent.click(await screen.findByText("做成卡片"));
    const dialog = await screen.findByRole("dialog");
    const [, answer] = Array.from(
      dialog.querySelectorAll<HTMLTextAreaElement>("textarea"),
    );
    expect(answer.value).toBe(TEXT.slice(0, 20).trim());
  });

  it("ignores short selections and the disabled mode", async () => {
    settings(false);
    renderIt();
    selectParagraph(screen.getByTestId("text"), 0, 3);
    await act(async () => {});
    expect(screen.queryByText("做成卡片")).toBeNull();
    cleanup();

    renderIt(true);
    selectParagraph(screen.getByTestId("text"));
    await act(async () => {});
    expect(screen.queryByText("做成卡片")).toBeNull();
  });

  it("Escape closes only the card dialog", async () => {
    settings(false);
    const outer = vi.fn();
    document.addEventListener("keydown", outer);
    renderIt();
    selectParagraph(screen.getByTestId("text"));
    fireEvent.click(await screen.findByText("做成卡片"));
    await screen.findByRole("dialog");
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(outer).not.toHaveBeenCalled();
    document.removeEventListener("keydown", outer);
  });
});
