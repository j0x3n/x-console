// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter } from "react-router";
import Sidebar, { useCurrentNavPanel } from "./Sidebar";
import { registerNavBadge } from "../../lib/navBadges";
import { usePreferencesStore } from "../../stores/preferences-store";

const access = vi.hoisted(() => ({ hidden: new Set<string>() }));
vi.mock("../../api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/core")>()),
  useAuthStatus: () => ({ data: { username: "test" } }),
  useLogout: () => ({ mutate: vi.fn() }),
}));
vi.mock("../../app/modules", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../app/modules")>()),
  useModules: () => ({
    pending: false,
    has: (id: string | null) => !id || !access.hidden.has(id),
  }),
}));
vi.mock("../../lib/navChildren", () => ({
  useNavChildren: () => ({ "/habits": () => <div>习惯自己的菜单</div> }),
}));
import.meta.glob("../../features/*/i18n.ts", { eager: true });

afterEach(() => {
  cleanup();
  access.hidden.clear();
});

function Probe({ onNavigate }: { onNavigate: () => void }) {
  const { Panel, panelLabel } = useCurrentNavPanel();
  return (
    <>
      <h2>{panelLabel}</h2>
      {Panel && <Panel onNavigate={onNavigate} />}
    </>
  );
}
function show(path: string) {
  const onNavigate = vi.fn();
  const view = render(
    <MemoryRouter initialEntries={[path]}>
      <Probe onNavigate={onNavigate} />
    </MemoryRouter>,
  );
  return { ...view, onNavigate };
}

describe("XC-57 合并菜单", () => {
  it("成员模块延迟注册角标时，切换角标 Hook 不复用旧组件", () => {
    const previous = usePreferencesStore.getState().navPins;
    function useRemindersBadge() {
      const [count] = useState(1);
      return { count, title: "提醒数量" };
    }
    function useCalendarBadge() {
      const [count] = useState(2);
      useState(0);
      return { count, title: "日程数量" };
    }
    registerNavBadge("/reminders", useRemindersBadge);
    usePreferencesStore.setState({ navPins: ["/calendar"] });
    try {
      render(
        <MemoryRouter>
          <Sidebar
            mobileOpen={false}
            setMobileOpen={vi.fn()}
            openPalette={vi.fn()}
          />
        </MemoryRouter>,
      );
      expect(screen.getByTitle("提醒数量").textContent).toBe("1");
      act(() => registerNavBadge("/calendar", useCalendarBadge));
      expect(screen.getByTitle("日程数量").textContent).toBe("2");
    } finally {
      cleanup();
      usePreferencesStore.setState({ navPins: previous });
    }
  });

  it("个人标题和两列成员列表，下面保留当前功能菜单", () => {
    const view = show("/habits");
    expect(screen.getByRole("heading").textContent).toBe("Personal");
    expect(view.container.querySelector(".nav-merge.two-columns")).toBeTruthy();
    expect(screen.getAllByRole("link")).toHaveLength(9);
    expect(
      screen
        .getByRole("link", { name: "习惯", exact: true })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(screen.getByText("习惯自己的菜单")).toBeTruthy();
    fireEvent.click(screen.getByRole("link", { name: "菜谱" }));
    expect(view.onNavigate).toHaveBeenCalled();
  });

  it("日程成员单列，提醒页高亮提醒，标题仍是日程入口", () => {
    const view = show("/reminders");
    expect(screen.getByRole("heading").textContent).toBe("Schedule & focus");
    expect(view.container.querySelector(".nav-merge.two-columns")).toBeNull();
    expect(screen.getAllByRole("link")).toHaveLength(4);
    expect(
      screen
        .getByRole("link", { name: "提醒", exact: true })
        .getAttribute("aria-current"),
    ).toBe("page");
  });

  it("隐藏设备入口时仍可显示可见成员，标题用设备", () => {
    access.hidden = new Set(["servers", "pc", "home", "screentime"]);
    show("/router");
    expect(screen.getByRole("heading").textContent).toBe("Devices");
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(screen.getByRole("link", { name: "路由器" })).toBeTruthy();
  });
});
