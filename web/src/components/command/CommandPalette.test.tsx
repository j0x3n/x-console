// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ALL_MODULES } from "../../app/modules";
import { registerCommands } from "../../lib/commands";
import CommandPalette from "./CommandPalette";

const capture = vi.fn();
registerCommands([
  {
    id: "test.capture",
    title: "存成笔记",
    group: "笔记",
    prefix: ">",
    run: ({ text }) => capture(text),
  },
  { id: "test.other", title: "新建提醒", group: "提醒", run: () => {} },
]);

function renderPalette(onClose = () => {}) {
  // 可用模块的接口回 404：旧服务端，全部模块都能用（B57）
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["app", "modules"], ALL_MODULES);
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <CommandPalette open onClose={onClose} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return screen.getByPlaceholderText(/搜索|Search/);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  capture.mockReset();
});

describe("CommandPalette prefixes", () => {
  it("passes the text after > to the command", async () => {
    const onClose = vi.fn();
    const input = renderPalette(onClose);
    fireEvent.change(input, { target: { value: "> 买牛奶" } });
    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(screen.getByRole("option").textContent).toContain(
      "存成笔记：买牛奶",
    );
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(capture).toHaveBeenCalledWith("买牛奶"));
    expect(onClose).toHaveBeenCalled();
  });

  it("fills in the prefix when picked without text", () => {
    const input = renderPalette() as HTMLInputElement;
    fireEvent.change(input, { target: { value: "存成" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(input.value).toBe("> ");
    expect(capture).not.toHaveBeenCalled();
  });

  it("shows the prefix hint and keeps normal search working", () => {
    const input = renderPalette();
    expect(screen.getByText(">")).toBeTruthy();
    fireEvent.change(input, { target: { value: "提醒" } });
    const texts = screen.getAllByRole("option").map((o) => o.textContent);
    expect(texts.some((x) => x?.includes("新建提醒"))).toBe(true);
  });
});
