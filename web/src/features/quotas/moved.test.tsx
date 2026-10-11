// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { routes } from "./routes";
import { navItems } from "../../app/nav";

// B152：AI 额度放进了监控，旧地址跳过去，左栏集成里不再有它。
function Where() {
  return <p>at {useLocation().pathname}</p>;
}

describe("AI 额度的位置", () => {
  it("旧地址 /quotas 跳到 /monitoring/quotas", () => {
    render(
      <MemoryRouter initialEntries={["/quotas"]}>
        <Routes>
          <Route path={routes[0].path} element={routes[0].element} />
          <Route path="/monitoring/quotas" element={<Where />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByText("at /monitoring/quotas")).toBeTruthy();
  });

  it("左栏里没有单独的入口", () => {
    expect(navItems.some((i) => i.path === "/quotas")).toBe(false);
  });
});
