// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { navItems } from "../../app/nav";
import Launcher from "./Launcher";
import { buildEntries } from "./navEntries";

// 各模块的名字登记在自己的 i18n.ts 里，应用启动时都会加载
import.meta.glob("../../features/*/i18n.ts", { eager: true });

afterEach(cleanup);

const entries = buildEntries(navItems);

function renderLauncher(pins: string[]) {
  const props = {
    onOpen: vi.fn(),
    onTogglePin: vi.fn(),
    onClose: vi.fn(),
  };
  render(
    <Launcher
      entries={entries}
      pinned={(path) => pins.includes(path)}
      {...props}
    />,
  );
  return props;
}

describe("启动台（B179）", () => {
  it("按分组列出全部入口，合并组写出包含哪些", () => {
    renderLauncher([]);
    expect(screen.getByRole("dialog", { name: "全部功能" })).toBeTruthy();
    expect(screen.getByText("工作")).toBeTruthy();
    expect(screen.getByText("笔记 · 稍后阅读 · 复习卡片")).toBeTruthy();
    expect(screen.getByText("家")).toBeTruthy();
  });

  it("点图钉固定或取消，点入口打开", () => {
    const props = renderLauncher(["/mail"]);
    const unpin = screen.getByRole("button", { name: "从左栏移除: 邮件" });
    expect(unpin.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(unpin);
    expect(props.onTogglePin).toHaveBeenCalledWith("/mail");
    fireEvent.click(screen.getByRole("button", { name: "固定到左栏: 联系人" }));
    expect(props.onTogglePin).toHaveBeenCalledWith("/contacts");
    fireEvent.click(screen.getByText("联系人"));
    expect(props.onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ path: "/contacts" }),
    );
  });

  it("搜索按名字过滤，组里的功能名也能搜到，回车打开第一个", () => {
    const props = renderLauncher([]);
    const input = screen.getByRole("textbox", { name: "搜索功能" });
    fireEvent.change(input, { target: { value: "复习" } });
    expect(screen.getByText("笔记")).toBeTruthy();
    expect(screen.queryByText("邮件")).toBeNull();
    fireEvent.submit(input.closest("form")!);
    expect(props.onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ path: "/notes" }),
    );
    fireEvent.change(input, { target: { value: "没有这个" } });
    expect(screen.getByText("没有匹配的功能")).toBeTruthy();
  });

  it("Esc 关闭", () => {
    const props = renderLauncher([]);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(props.onClose).toHaveBeenCalled();
  });
});
