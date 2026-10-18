// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { navItems } from "../../app/nav";
import Launcher from "./Launcher";
import { buildEntries, isPinned } from "./navEntries";

// 各模块的名字登记在自己的 i18n.ts 里，应用启动时都会加载
import.meta.glob("../../features/*/i18n.ts", { eager: true });

afterEach(cleanup);

const entries = buildEntries(navItems);

function renderLauncher(pins: string[], showOrder = false) {
  const props = {
    onOpen: vi.fn(),
    onTogglePin: vi.fn(),
    onClose: vi.fn(),
    onChangePins: vi.fn(),
  };
  render(
    <Launcher
      entries={entries}
      pins={showOrder ? pins : undefined}
      pinned={(path) => isPinned(pins, path)}
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
    expect(screen.getByText("笔记 · 稍后阅读 · 复习卡片 · 资讯")).toBeTruthy();
    expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual([
      "工作",
      "个人",
      "设备",
    ]);
    expect(screen.queryByText("集成")).toBeNull();
  });

  it("点图钉固定或取消，点入口打开", () => {
    const props = renderLauncher(["/mail"]);
    const unpin = screen.getByRole("button", { name: "从左栏移除: 邮件" });
    expect(unpin.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(unpin);
    expect(props.onTogglePin).toHaveBeenCalledWith("/mail");
    fireEvent.click(screen.getByRole("button", { name: "固定到左栏: 个人" }));
    expect(props.onTogglePin).toHaveBeenCalledWith("/habits");
    fireEvent.click(screen.getByText("个人", { selector: ".launcher-name" }));
    expect(props.onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ path: "/habits" }),
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

  it.each([
    ["菜谱", "/habits"],
    ["仓库", "/coding"],
    ["资讯", "/notes"],
  ])("搜索成员 %s 找到所属入口", (query, path) => {
    const props = renderLauncher([]);
    const input = screen.getByRole("textbox", { name: "搜索功能" });
    fireEvent.change(input, { target: { value: query } });
    expect(document.querySelectorAll(".launcher-tile")).toHaveLength(1);
    fireEvent.submit(input.closest("form")!);
    expect(props.onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ path }),
    );
  });

  it("旧固定项在顺序列表只显示一次，支持移动和移除", () => {
    const props = renderLauncher(
      ["/mail", "/router", "/home", "/servers", "/contacts"],
      true,
    );
    const list = screen.getByRole("list");
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((item) => item.getAttribute("aria-label")),
    ).toEqual(["邮件", "设备", "个人"]);
    expect(
      screen
        .getByRole("button", { name: "从左栏移除: 设备" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    fireEvent.keyDown(within(list).getByRole("listitem", { name: "设备" }), {
      key: "ArrowLeft",
    });
    expect(props.onChangePins).toHaveBeenLastCalledWith([
      "/servers",
      "/mail",
      "/habits",
    ]);
    fireEvent.click(
      within(list).getByRole("button", { name: "从左栏顺序里移除: 设备" }),
    );
    expect(props.onChangePins).toHaveBeenLastCalledWith(["/mail", "/habits"]);
  });

  it("隐藏成员不出现在搜索结果", () => {
    render(
      <Launcher
        entries={buildEntries(navItems.filter((n) => n.path !== "/github"))}
        pinned={() => false}
        onOpen={vi.fn()}
        onTogglePin={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByRole("textbox", { name: "搜索功能" }), {
      target: { value: "仓库" },
    });
    expect(screen.getByText("没有匹配的功能")).toBeTruthy();
  });
});
