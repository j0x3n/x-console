import { describe, expect, it } from "vitest";
import {
  lastPathFor,
  moduleRoot,
  readScroll,
  rememberPath,
  saveScroll,
} from "./useKeepScroll";

describe("B80 keep place", () => {
  it("finds the module of a path", () => {
    expect(moduleRoot("/notes/12")).toBe("/notes");
    expect(moduleRoot("/")).toBeNull();
    expect(moduleRoot("/settings/backup")).toBeNull();
  });

  it("goes back to the last page of a module, or its root when already there", () => {
    rememberPath("/notes/12", "?tag=work");
    expect(lastPathFor("/notes", "/servers")).toBe("/notes/12?tag=work");
    expect(lastPathFor("/notes", "/notes/12")).toBe("/notes");
    expect(lastPathFor("/drive", "/servers")).toBe("/drive");
  });

  it("remembers scroll positions", () => {
    saveScroll("notes.list:", 420.4);
    expect(readScroll("notes.list:")).toBe(420);
    expect(readScroll("nope")).toBeUndefined();
  });

  it("模块记忆不保存设置弹窗，旧会话中的弹窗参数也会清除", () => {
    rememberPath("/projects/5", "?board=2&settings=git&section=accounts");
    expect(lastPathFor("/projects", "/notes")).toBe("/projects/5?board=2");
  });
});
