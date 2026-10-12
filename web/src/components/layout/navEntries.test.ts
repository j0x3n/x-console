import { describe, expect, it } from "vitest";
import { defaultNavPins, entryOf, navItems } from "../../app/nav";
import {
  buildEntries,
  isPinned,
  movePin,
  newSeparator,
  railEntries,
  railItems,
  togglePin,
} from "./navEntries";

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

  it("分隔线：去掉开头、结尾和连着的，编号不重复", () => {
    const keys = (pins: string[]) =>
      railItems(all, pins, "/").map((i) =>
        i.kind === "sep" ? "-" : i.entry.path,
      );
    expect(keys(["/-/1", "/notes", "/-/2", "/-/3", "/mail", "/-/4"])).toEqual([
      "/notes",
      "-",
      "/mail",
    ]);
    // 中间的入口不认识（比如被隐藏），两条线只留一条
    expect(keys(["/notes", "/-/1", "/nope", "/-/2", "/mail"])).toEqual([
      "/notes",
      "-",
      "/mail",
    ]);
    expect(newSeparator(["/-/1", "/-/3"])).toBe("/-/2");
  });

  it("移动固定项", () => {
    expect(movePin(["/a", "/b", "/c"], 0, 2)).toEqual(["/b", "/c", "/a"]);
    expect(movePin(["/a", "/b", "/c"], 2, 0)).toEqual(["/c", "/a", "/b"]);
  });
});
