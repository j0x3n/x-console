// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import InstallButton from "./InstallButton";
import InstallCard from "./InstallCard";
import { isIOS, useInstall } from "./install";

function fakePrompt(outcome: "accepted" | "dismissed") {
  const e = new Event("beforeinstallprompt") as Event & {
    prompt: () => Promise<void>;
    userChoice: Promise<{ outcome: typeof outcome }>;
  };
  e.prompt = vi.fn(() => Promise.resolve());
  e.userChoice = Promise.resolve({ outcome });
  return e;
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  useInstall.setState({ prompt: null, installed: false, dismissed: false });
});

describe("install", () => {
  it("detects iPhone and iPad", () => {
    expect(
      isIOS("Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)"),
    ).toBe(true);
    expect(isIOS("Mozilla/5.0 (Windows NT 10.0; Win64; x64)")).toBe(false);
  });

  it("shows the header button once the browser allows installing", async () => {
    render(<InstallButton />);
    expect(screen.queryByRole("button")).toBeNull();
    const e = fakePrompt("accepted");
    act(() => {
      window.dispatchEvent(e);
    });
    const button = screen.getByRole("button", { name: "安装应用" });
    await act(async () => {
      fireEvent.click(button);
    });
    expect(e.prompt).toHaveBeenCalled();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("can hide the header button from settings", () => {
    act(() => {
      window.dispatchEvent(fakePrompt("dismissed"));
    });
    render(
      <>
        <InstallButton />
        <InstallCard />
      </>,
    );
    expect(screen.getAllByRole("button", { name: /安装应用/ })).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "页头不再显示" }));
    expect(screen.getAllByRole("button", { name: /安装应用/ })).toHaveLength(1);
    expect(localStorage.getItem("xc.pwa.dismissed")).toBe("1");
  });
});
