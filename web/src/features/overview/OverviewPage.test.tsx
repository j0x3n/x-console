// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  layout: {
    cards: [
      { id: "greeting", visible: true, order: 0 },
      { id: "issues", visible: true, order: 1 },
    ],
  },
}));

vi.mock("../../contexts/LanguageContext", () => ({
  useT: () => (text: string) => text,
}));
vi.mock("../../hooks/useToast", () => ({ toast: vi.fn() }));
vi.mock("./api", () => ({
  useDashboardLayout: () => ({
    isPending: false,
    isError: false,
    data: mocks.layout,
  }),
  useSaveDashboardLayout: () => ({ isPending: false, mutate: mocks.save }),
}));
vi.mock("./cards", () => ({
  cardLabels: { greeting: "Greeting", issues: "Issues" },
  cardComponents: {
    greeting: () => <span>正常卡片</span>,
    issues: () => {
      throw new Error("卡片故障");
    },
  },
}));

import OverviewPage from "./OverviewPage";

beforeEach(() => mocks.save.mockReset());
afterEach(cleanup);

it("keeps other cards visible when one card throws", () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    render(<OverviewPage />);
    expect(screen.getByText("正常卡片")).toBeTruthy();
    expect(screen.getByText(/卡片加载失败：卡片故障/)).toBeTruthy();
  } finally {
    log.mockRestore();
  }
});

it("saves visibility and card order", () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    render(<OverviewPage />);
    fireEvent.click(screen.getByText("Edit overview"));
    fireEvent.click(screen.getByRole("checkbox", { name: "Issues" }));
    fireEvent.click(screen.getByRole("button", { name: "上移 Issues" }));
    fireEvent.click(screen.getByText("Save overview"));
    expect(mocks.save).toHaveBeenCalledWith(
      {
        cards: [
          { id: "issues", visible: false, order: 0 },
          { id: "greeting", visible: true, order: 1 },
        ],
      },
      expect.any(Object),
    );
  } finally {
    log.mockRestore();
  }
});
