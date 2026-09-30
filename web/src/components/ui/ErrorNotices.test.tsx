// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import ErrorNotices from "./ErrorNotices";
import { reportError, useErrorStore } from "../../lib/errors";

beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  useErrorStore.setState({ notices: [], history: [] });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ErrorNotices", () => {
  it("shows at most 3 and folds the rest", () => {
    for (let i = 0; i < 5; i++) reportError(new Error(`错误 ${i}`));
    render(<ErrorNotices />);
    expect(screen.getAllByRole("alert")).toHaveLength(3);
    fireEvent.click(screen.getByText("还有 2 条报错"));
    expect(screen.getAllByRole("alert")).toHaveLength(5);
    fireEvent.click(screen.getByText("全部关闭"));
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });

  it("copies the full detail", async () => {
    const writeText = vi.fn(async () => undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    reportError(new Error("boom"), { title: "保存失败" });
    render(<ErrorNotices />);
    fireEvent.click(screen.getByText("复制"));
    await screen.findByText("已复制");
    expect(writeText).toHaveBeenCalledWith(
      useErrorStore.getState().notices[0].detail,
    );
  });

  it("closes one notice", () => {
    reportError(new Error("boom"));
    render(<ErrorNotices />);
    fireEvent.click(screen.getByLabelText("关闭"));
    expect(useErrorStore.getState().notices).toHaveLength(0);
  });
});
