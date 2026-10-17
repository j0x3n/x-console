// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";

const shot = vi.hoisted(() => ({ calls: 0 }));
vi.mock("modern-screenshot", () => ({
  domToBlob: async () => {
    shot.calls++;
    return new Blob(["png"], { type: "image/png" });
  },
}));

import ImageExportDialog from "./ImageExportDialog";
import "../i18n";

const note = {
  title: "周末计划",
  body: "去爬山\n\n:::hidden 密码\n不要给别人看\n:::\n\n带水",
  tags: ["生活"],
  color: "yellow",
  updatedAt: "2026-10-17T02:00:00Z",
};

beforeEach(() => {
  vi.useFakeTimers();
  URL.createObjectURL = () => "blob:preview";
  URL.revokeObjectURL = () => undefined;
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  localStorage.clear();
  shot.calls = 0;
});

function open(selection = "") {
  return render(
    <MemoryRouter>
      <ImageExportDialog note={note} selection={selection} onClose={() => {}} />
    </MemoryRouter>,
  );
}

const sheet = () => document.querySelector(".notes-img-sheet")!;

describe("B242 导出图片弹窗", () => {
  it("没选中内容时“选中的部分”是灰的，默认导出整篇", () => {
    open();
    expect(
      (screen.getByRole("button", { name: "选中的部分" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      screen.getByRole("button", { name: "整篇" }).getAttribute("aria-pressed"),
    ).toBe("true");
    expect(sheet().textContent).toContain("去爬山");
  });

  it("有选中内容时默认只导出选中的部分", () => {
    open("带水");
    expect(
      screen
        .getByRole("button", { name: "选中的部分" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(sheet().textContent).toContain("带水");
    expect(sheet().textContent).not.toContain("去爬山");
  });

  it("默认带标题和日期，背景跟随笔记颜色", () => {
    open();
    expect(sheet().className).toContain("note-bg-yellow");
    expect(sheet().querySelector(".notes-img-title")?.textContent).toBe(
      "周末计划",
    );
    expect(sheet().querySelector(".notes-img-meta")?.textContent).toContain(
      "2026-10-17",
    );
    expect(sheet().textContent).not.toContain("#生活");
  });

  it("取消标题后预览里没有标题，选项记下来", () => {
    open();
    fireEvent.click(screen.getByLabelText("标题"));
    expect(sheet().querySelector(".notes-img-title")).toBeNull();
    expect(
      JSON.parse(localStorage.getItem("xc.notes.imageExport")!).title,
    ).toBe(false);
  });

  it("不勾隐藏块时整块不出现，勾了直接展开", () => {
    open();
    expect(sheet().textContent).not.toContain("不要给别人看");
    expect(sheet().querySelector(".xc-md-hidden")).toBeNull();
    fireEvent.click(screen.getByLabelText("隐藏块的内容"));
    expect(sheet().textContent).toContain("不要给别人看");
    expect(sheet().querySelector("button")).toBeNull();
  });

  it("生成以后显示预览，可以保存", async () => {
    open();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(shot.calls).toBe(1);
    expect(screen.getByAltText("图片预览").getAttribute("src")).toBe(
      "blob:preview",
    );
    expect(
      (screen.getByRole("button", { name: /保存图片/ }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });
});
