// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { InstallHelpDialog, InstallMenuItem } from "./InstallMenu";
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
  useInstall.setState({ prompt: null, installed: false, helpOpen: false });
});

describe("install", () => {
  it("detects iPhone and iPad", () => {
    expect(
      isIOS("Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)"),
    ).toBe(true);
    expect(isIOS("Mozilla/5.0 (Windows NT 10.0; Win64; x64)")).toBe(false);
  });

  it("installs from the profile menu when the browser allows it", async () => {
    const onDone = vi.fn();
    const e = fakePrompt("accepted");
    act(() => {
      window.dispatchEvent(e);
    });
    render(<InstallMenuItem onDone={onDone} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: "安装应用" }));
    });
    expect(e.prompt).toHaveBeenCalled();
    expect(onDone).toHaveBeenCalled();
  });

  it("explains how to install when the browser cannot do it directly", () => {
    render(
      <>
        <InstallMenuItem onDone={() => {}} />
        <InstallHelpDialog />
      </>,
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "安装应用" }));
    expect(screen.getByRole("dialog", { name: "安装应用" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "知道了" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("hides the menu item once installed", () => {
    useInstall.setState({ installed: true });
    render(<InstallMenuItem onDone={() => {}} />);
    expect(screen.queryByRole("menuitem")).toBeNull();
  });
});
