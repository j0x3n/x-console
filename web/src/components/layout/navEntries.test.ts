import { describe, expect, it } from "vitest";
import { defaultNavPins, entryOf, navItems } from "../../app/nav";
import { buildEntries, isPinned, railEntries, togglePin } from "./navEntries";

const all = buildEntries(navItems);
const paths = (list: { path: string }[]) => list.map((e) => e.path);

describe("左栏入口（B179）", () => {
  it("合并三组，今日不在里面", () => {
    expect(all.length).toBe(navItems.length - 1 - 7);
    expect(paths(all)).not.toContain("/");
    const notes = all.find((e) => e.path === "/notes")!;
    expect(paths(notes.members)).toEqual([
      "/notes",
      "/readlater",
      "/cards",
      "/ask",
    ]);
    expect(all.find((e) => e.path === "/home")?.label).toBe("Home and network");
    expect(paths(all)).not.toContain("/readlater");
    expect(paths(all)).not.toContain("/router");
  });

  it("组里的功能都归到组的入口", () => {
    expect(entryOf("/cards")).toBe("/notes");
    expect(entryOf("/checkins")).toBe("/habits");
    expect(entryOf("/router")).toBe("/home");
    expect(entryOf("/mail")).toBe("/mail");
  });

  it("隐藏的功能不出现，组里只剩看得见的", () => {
    const visible = navItems.filter(
      (n) => n.path !== "/readlater" && n.path !== "/home",
    );
    const entries = buildEntries(visible);
    expect(
      paths(entries.find((e) => e.path === "/notes")!.members),
    ).not.toContain("/readlater");
    // 入口自己被隐藏时，组还在，用剩下的功能
    expect(paths(entries.find((e) => e.path === "/home")!.members)).toEqual([
      "/router",
    ]);
  });

  it("没设置时用默认固定的入口", () => {
    expect(paths(railEntries(all, null, "/"))).toEqual(defaultNavPins);
  });

  it("当前入口没固定时临时排在最后，在组里也算", () => {
    const rail = railEntries(all, ["/projects"], "/checkins");
    expect(paths(rail)).toEqual(["/projects", "/habits"]);
    // 已经固定的不重复
    expect(paths(railEntries(all, ["/notes"], "/cards"))).toEqual(["/notes"]);
  });

  it("不认识的地址忽略", () => {
    expect(paths(railEntries(all, ["/nope", "/mail"], "/"))).toEqual(["/mail"]);
  });

  it("固定和取消固定", () => {
    const pins = togglePin(null, "/contacts");
    expect(pins).toEqual([...defaultNavPins, "/contacts"]);
    expect(isPinned(pins, "/contacts")).toBe(true);
    expect(togglePin(pins, "/projects")).not.toContain("/projects");
    expect(isPinned(null, "/projects")).toBe(true);
  });
});
