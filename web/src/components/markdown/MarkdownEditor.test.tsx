// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useToastStore } from "../../hooks/useToast";
import { useErrorStore } from "../../lib/errors";
import MarkdownEditor from "./MarkdownEditor";

let latest = "";
function Editor() {
  const [value, setValue] = useState("开头");
  latest = value;
  return (
    <MarkdownEditor
      label="描述"
      value={value}
      onChange={setValue}
      uploadScope="projects"
    />
  );
}

const png = () => new File(["x"], "shot.png", { type: "image/png" });
const paste = (files: File[], text = "") =>
  fireEvent.paste(screen.getByLabelText("描述"), {
    clipboardData: { files, getData: () => text },
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  useToastStore.setState({ current: null });
  useErrorStore.setState({ notices: [], history: [] });
});

describe("MarkdownEditor image paste", () => {
  it("uploads a pasted image and replaces the placeholder", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(
          JSON.stringify({
            id: 5,
            name: "shot.png",
            mime: "image/png",
            size: 1,
            url: "/api/v1/files/5",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<Editor />);
    paste([png()]);
    expect(latest).toContain("![上传中 shot.png");
    await waitFor(() =>
      expect(latest).toContain("![shot.png](/api/v1/files/5)"),
    );
    expect(latest).not.toContain("上传中");
    expect(String(fetchMock.mock.calls[0][0])).toBe(
      "/api/v1/files?scope=projects",
    );
  });

  it("removes the placeholder and says so when uploads are not live", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 404 })),
    );
    render(<Editor />);
    paste([png()]);
    await waitFor(() => expect(latest).toBe("开头"));
    // 报错不走普通提示，进报错列表（B41）。
    expect(useErrorStore.getState().notices[0]?.message).toBe(
      "图片上传还没上线",
    );
  });

  it("pastes text normally when the clipboard also has text", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(<Editor />);
    paste([png()], "表格里的文字");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses files that are not images", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(<Editor />);
    paste([new File(["x"], "a.pdf", { type: "application/pdf" })]);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(latest).toBe("开头");
  });
});

describe("AI 润色（B56）", () => {
  it("按场景润色，替换后改掉编辑框内容，Esc 不影响外层", async () => {
    const calls: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init: RequestInit) => {
        calls.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify({ text: "润色后" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const outer = vi.fn();
    function Polished() {
      const [value, setValue] = useState("原来的");
      latest = value;
      return (
        <div onKeyDown={outer}>
          <MarkdownEditor
            label="描述"
            value={value}
            onChange={setValue}
            polish="card"
          />
        </div>
      );
    }
    render(<Polished />);
    fireEvent.click(screen.getByRole("button", { name: "AI 润色" }));
    fireEvent.keyDown(screen.getByLabelText("润色要求"), { key: "a" });
    expect(outer).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /开始润色/ }));
    fireEvent.click(await screen.findByRole("button", { name: "替换正文" }));
    await waitFor(() => expect(latest).toBe("润色后"));
    expect(calls[0]).toEqual({ text: "原来的", scene: "card" });
  });
});
