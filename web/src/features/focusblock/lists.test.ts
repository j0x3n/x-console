import { describe, expect, it } from "vitest";
import {
  addApps,
  appKey,
  joinLines,
  parseApps,
  parseDomains,
  sameList,
} from "./lists";

describe("专注屏蔽的名单", () => {
  it("网址按换行、逗号和空格分开，去掉空项和重复项", () => {
    expect(
      parseDomains("youtube.com\n\n bilibili.com, weibo.com  youtube.com"),
    ).toEqual(["youtube.com", "bilibili.com", "weibo.com"]);
    expect(parseDomains("  \n ")).toEqual([]);
  });

  it("网址可以直接粘贴整个地址，整理交给服务端", () => {
    expect(parseDomains("https://www.youtube.com/watch?v=1")).toEqual([
      "https://www.youtube.com/watch?v=1",
    ]);
  });

  it("程序名按换行和逗号分开，名字里的空格留着", () => {
    expect(parseApps("steam\nGoogle Chrome, vlc")).toEqual([
      "steam",
      "Google Chrome",
      "vlc",
    ]);
  });

  it("程序名比较时不分大小写，也不看 .exe", () => {
    expect(appKey(" Steam.EXE ")).toBe("steam");
    expect(parseApps("Steam.exe\nsteam\nSTEAM")).toEqual(["Steam.exe"]);
  });

  it("导入的程序只加没有的", () => {
    const r = addApps("steam\nvlc.exe", ["Steam.exe", "VLC", "spotify", "qq"]);
    expect(r.added).toBe(2);
    expect(r.text).toBe("steam\nvlc.exe\nspotify\nqq");
    expect(addApps(r.text, ["spotify"]).added).toBe(0);
    expect(addApps("", ["a", "b"]).text).toBe("a\nb");
  });

  it("比较两个名单要顺序也一样", () => {
    expect(sameList(["a", "b"], ["a", "b"])).toBe(true);
    expect(sameList(["a", "b"], ["b", "a"])).toBe(false);
    expect(sameList(["a"], ["a", "b"])).toBe(false);
    expect(joinLines(["a", "b"])).toBe("a\nb");
  });
});
