// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  render,
  screen,
  fireEvent,
} from "@testing-library/react";
import { useDraft, useKeptState } from "./useDraft";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function Comment({ id, onSent }: { id: string; onSent?: () => void }) {
  const [body, setBody, clear] = useDraft(`test.comment.${id}`);
  return (
    <>
      <input
        aria-label="comment"
        value={body}
        onChange={(e) => setBody(e.target.value)}
      />
      <button
        onClick={() => {
          clear();
          onSent?.();
        }}
      >
        send
      </button>
    </>
  );
}

function Tabs() {
  const [tab, setTab] = useKeptState<"a" | "b">("test.tab", "a");
  return (
    <button aria-label="tab" onClick={() => setTab(tab === "a" ? "b" : "a")}>
      {tab}
    </button>
  );
}

const input = () => screen.getByLabelText("comment") as HTMLInputElement;

describe("useDraft (B249)", () => {
  it("keeps a half-written draft after unmounting", () => {
    const first = render(<Comment id="1" />);
    fireEvent.change(input(), { target: { value: "写了一半" } });
    first.unmount();
    render(<Comment id="1" />);
    expect(input().value).toBe("写了一半");
  });

  it("keys drafts by object", () => {
    const view = render(<Comment id="2" />);
    fireEvent.change(input(), { target: { value: "第二张卡片" } });
    view.rerender(<Comment id="3" />);
    expect(input().value).toBe("");
    view.rerender(<Comment id="2" />);
    expect(input().value).toBe("第二张卡片");
  });

  it("clears the draft after sending", () => {
    const first = render(<Comment id="4" />);
    fireEvent.change(input(), { target: { value: "发出去" } });
    fireEvent.click(screen.getByText("send"));
    expect(input().value).toBe("");
    first.unmount();
    render(<Comment id="4" />);
    expect(input().value).toBe("");
  });

  it("survives a page reload through sessionStorage", async () => {
    vi.useFakeTimers();
    const first = render(<Comment id="5" />);
    fireEvent.change(input(), { target: { value: "刷新也在" } });
    act(() => vi.runAllTimers());
    first.unmount();
    expect(sessionStorage.getItem("xc.drafts")).toContain("刷新也在");
    vi.useRealTimers();

    // 重新加载模块，等于刷新页面：内存清空，从 sessionStorage 读回来
    vi.resetModules();
    const fresh = await import("./useDraft");
    function Reloaded() {
      const [body] = fresh.useDraft("test.comment.5");
      return <span>{body}</span>;
    }
    render(<Reloaded />);
    expect(screen.getByText("刷新也在")).toBeTruthy();
  });
});

describe("useKeptState (B249)", () => {
  it("keeps the picked tab after leaving and coming back", () => {
    const first = render(<Tabs />);
    fireEvent.click(screen.getByLabelText("tab"));
    expect(screen.getByLabelText("tab").textContent).toBe("b");
    first.unmount();
    render(<Tabs />);
    expect(screen.getByLabelText("tab").textContent).toBe("b");
  });
});
