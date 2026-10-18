import { describe, expect, it } from "vitest";
import {
  defaultNavPins,
  entryOf,
  navEntryIcons,
  navItems,
  navMerges,
} from "../../app/nav";
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
  it("XC-57 合并为 11 个入口，成员顺序与导航一致", () => {
    expect(paths(all)).toEqual([
      "/projects",
      "/coding",
      "/notes",
      "/ask",
      "/mail",
      "/calendar",
      "/music",
      "/drive",
      "/servers",
      "/habits",
      "/monitoring",
    ]);
    const groups = {
      "/coding": ["/coding", "/github"],
      "/notes": ["/notes", "/readlater", "/cards", "/feeds"],
      "/calendar": ["/calendar", "/reminders", "/plan", "/inbox"],
      "/servers": ["/servers", "/pc", "/router", "/home", "/screentime"],
      "/habits": [
        "/habits",
        "/goals",
        "/checkins",
        "/experiments",
        "/skills",
        "/documents",
        "/contacts",
        "/recipes",
        "/forecasts",
      ],
      "/monitoring": ["/monitoring", "/credentials", "/automations"],
    };
    expect(navMerges).toEqual(groups);
    for (const [entry, members] of Object.entries(groups)) {
      expect(paths(all.find((e) => e.path === entry)!.members)).toEqual(
        members,
      );
      expect(paths(navItems.filter((n) => members.includes(n.path)))).toEqual(
        members,
      );
      for (const member of members) expect(entryOf(member)).toBe(entry);
    }
    for (const [path, label] of [
      ["/servers", "Devices"],
      ["/habits", "Personal"],
      ["/monitoring", "Ops"],
    ]) {
      const entry = all.find((e) => e.path === path)!;
      expect(entry.label).toBe(label);
      expect(entry.icon).toBe(navEntryIcons[path]);
      expect(entry.icon).not.toBe(navItems.find((n) => n.path === path)!.icon);
    }
    expect(
      all.filter((e) => e.group === "machines").map((e) => e.path),
    ).toEqual(["/drive", "/servers", "/monitoring"]);
    expect(all.some((e) => e.group === "integrations")).toBe(false);
  });

  it("组里的功能都归到组的入口", () => {
    expect(entryOf("/cards")).toBe("/notes");
    expect(entryOf("/checkins")).toBe("/habits");
    expect(entryOf("/router")).toBe("/servers");
    expect(entryOf("/mail")).toBe("/mail");
  });

  it("隐藏的功能不出现，组里只剩看得见的", () => {
    const visible = navItems.filter(
      (n) => n.path !== "/readlater" && n.path !== "/servers",
    );
    const entries = buildEntries(visible);
    expect(
      paths(entries.find((e) => e.path === "/notes")!.members),
    ).not.toContain("/readlater");
    // 入口自己被隐藏时，组还在，用剩下的功能
    expect(paths(entries.find((e) => e.path === "/servers")!.members)).toEqual([
      "/pc",
      "/router",
      "/home",
      "/screentime",
    ]);
    expect(
      paths(
        buildEntries(navItems.filter((n) => entryOf(n.path) !== "/servers")),
      ),
    ).not.toContain("/servers");
  });

  it("没设置时用默认固定的入口", () => {
    expect(paths(railEntries(all, null, "/"))).toEqual(
      defaultNavPins.filter((p) => !p.startsWith("/-/")),
    );
    expect(defaultNavPins).toEqual([
      "/projects",
      "/coding",
      "/-/1",
      "/notes",
      "/ask",
      "/mail",
      "/calendar",
      "/music",
      "/drive",
      "/-/2",
      "/servers",
      "/habits",
      "/monitoring",
    ]);
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
    const pins = togglePin([], "/contacts");
    expect(pins).toEqual(["/habits"]);
    expect(isPinned(pins, "/contacts")).toBe(true);
    expect(togglePin(null, "/projects")).not.toContain("/projects");
    expect(isPinned(null, "/projects")).toBe(true);
  });

  it("旧固定项映射到入口并去重，保留原顺序和分隔线", () => {
    const pins = [
      "/github",
      "/coding",
      "/-/1",
      "/reminders",
      "/calendar",
      "/mail",
      "/router",
      "/home",
      "/servers",
      "/contacts",
    ];
    const copy = [...pins];
    expect(paths(railEntries(all, pins, "/reminders"))).toEqual([
      "/coding",
      "/calendar",
      "/mail",
      "/servers",
      "/habits",
    ]);
    expect(
      railItems(all, pins, "/router").map((i) =>
        i.kind === "sep" ? i.key : i.entry.path,
      ),
    ).toEqual(["/coding", "/-/1", "/calendar", "/mail", "/servers", "/habits"]);
    expect(isPinned(pins, "/servers")).toBe(true);
    expect(isPinned(pins, "/pc")).toBe(true);
    expect(togglePin(pins, "/servers")).toEqual([
      "/coding",
      "/-/1",
      "/calendar",
      "/mail",
      "/habits",
    ]);
    expect(togglePin(pins, "/reminders")).not.toContain("/calendar");
    expect(pins).toEqual(copy);
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
