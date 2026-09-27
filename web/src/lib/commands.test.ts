import { describe, expect, it } from "vitest";
import { matchPrefix, type Command } from "./commands";

const cmd = (id: string, prefix?: string): Command => ({
  id,
  title: id,
  group: "g",
  prefix,
  run: () => {},
});

describe("matchPrefix", () => {
  const list = [cmd("plain"), cmd("note", ">"), cmd("todo", ">>")];
  it("finds the command and the text after the prefix", () => {
    expect(matchPrefix("> 买牛奶 ", list)).toEqual({
      command: list[1],
      text: "买牛奶",
    });
    expect(matchPrefix(">", list)).toEqual({ command: list[1], text: "" });
    expect(matchPrefix("  >想法", list)?.text).toBe("想法");
  });
  it("prefers the longer prefix", () => {
    expect(matchPrefix(">> 写周报", list)?.command.id).toBe("todo");
  });
  it("returns null for normal searches", () => {
    expect(matchPrefix("笔记", list)).toBeNull();
    expect(matchPrefix("", list)).toBeNull();
  });
});
