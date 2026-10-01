// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import NavChildLinks from "./NavChildLinks";

const links = [
  { key: "p1", to: "/projects/XC", label: "X Console" },
  { key: "b1", to: "/projects/XC?board=1", label: "看板一", nested: true },
  { key: "b2", to: "/projects/XC?board=2", label: "看板二", nested: true },
  { key: "p2", to: "/projects/HOME", label: "家里" },
];

function show() {
  render(
    <MemoryRouter initialEntries={["/projects/XC?board=1"]}>
      <NavChildLinks
        links={links}
        limit={links.length}
        allTo="/projects"
        empty="还没有项目"
        onNavigate={() => {}}
      />
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("NavChildLinks (B68)", () => {
  it("keeps third-level links folded until the arrow is clicked", () => {
    show();
    expect(screen.getByText("X Console")).toBeTruthy();
    expect(screen.getByText("家里")).toBeTruthy();
    // 当前看的就是看板一，也不自动展开
    expect(screen.queryByText("看板一")).toBeNull();
    const arrow = screen.getByRole("button", { name: "展开 X Console" });
    expect(arrow.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(arrow);
    expect(screen.getByText("看板一")).toBeTruthy();
    expect(screen.getByText("看板二")).toBeTruthy();
    // 只有带子项的行有箭头
    expect(screen.queryByRole("button", { name: /家里/ })).toBeNull();

    // 刷新后保持展开
    cleanup();
    show();
    expect(screen.getByText("看板一")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "收起 X Console" }));
    expect(screen.queryByText("看板一")).toBeNull();
  });
});
