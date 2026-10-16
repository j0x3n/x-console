// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import "../i18n";
import IssueList from "./IssueList";
import type { Issue, IssueGroup } from "../logic";

const issue = (over: Partial<Issue>): Issue =>
  ({
    id: 1,
    key: "XC-1",
    title: "登录页改版",
    status: "todo",
    priority: 0,
    labels: [],
    members: [],
    updatedAt: new Date().toISOString(),
    ...over,
  }) as Issue;

const noop = () => {};

afterEach(cleanup);

describe("IssueList (B239)", () => {
  it("shows a header row and the priority as text", () => {
    const groups: IssueGroup[] = [
      {
        id: "todo",
        label: "Todo",
        status: "todo",
        issues: [issue({ priority: 2 })],
      },
      { id: "done", label: "Done", status: "done", issues: [] },
    ];
    const { container } = render(
      <IssueList
        groups={groups}
        selectedKey={null}
        onSelect={noop}
        onOpen={noop}
      />,
    );
    const head = container.querySelector(".projects-list-head");
    expect(head?.textContent).toContain("名称");
    expect(head?.textContent).toContain("优先级");
    expect(head?.textContent).toContain("更新时间");
    expect(head?.textContent).not.toContain("分类");
    // 组名带状态，色带的颜色靠它
    expect(
      container
        .querySelector(".projects-list-group > header")
        ?.getAttribute("data-status"),
    ).toBe("todo");
    expect(screen.getByText("高")).toBeTruthy();
  });

  it("adds a category column only when some issue has a category", () => {
    const groups: IssueGroup[] = [
      {
        id: "all",
        label: "All issues",
        issues: [issue({ categoryId: 7 }), issue({ key: "XC-2", id: 2 })],
      },
    ];
    const { container } = render(
      <IssueList
        groups={groups}
        categoryNames={new Map([[7, "前端 / 页面"]])}
        selectedKey={null}
        onSelect={noop}
        onOpen={noop}
        showProject
      />,
    );
    expect(container.querySelector(".projects-list")?.className).toContain(
      "with-category",
    );
    expect(screen.getByText("前端 / 页面")).toBeTruthy();
    // 两行的分类格子都在，没分类的是空的，列才对得齐
    expect(container.querySelectorAll(".projects-row-category")).toHaveLength(
      2,
    );
    // showProject 时不显示更新时间
    expect(container.querySelector(".projects-row-time")).toBeNull();
  });
});
