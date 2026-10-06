import { describe, expect, it } from "vitest";
import {
  deviceId,
  deviceTarget,
  kindLabel,
  mailScope,
  mailScopeId,
  mutedFromChecked,
  sameTargets,
  validKindPattern,
} from "./mutes";

describe("mute rules", () => {
  it("reads and writes scopes and device targets", () => {
    expect(mailScope(3)).toBe("mail:3");
    expect(mailScopeId("mail:3")).toBe(3);
    expect(mailScopeId("mail:x")).toBeNull();
    expect(mailScopeId("github:3")).toBeNull();
    expect(mailScopeId("")).toBeNull();
    expect(deviceTarget(5)).toBe("webpush:5");
    expect(deviceId("webpush:5")).toBe(5);
    expect(deviceId("webpush")).toBeNull();
    expect(deviceId("bark")).toBeNull();
  });

  it("names known kinds and leaves others as they are", () => {
    expect(kindLabel("mail.new")).toBe("New mail");
    expect(kindLabel("mail.*")).toBe("Mail");
    expect(kindLabel("*")).toBe("All notifications");
    expect(kindLabel("host.alert*")).toBeNull();
  });

  it("checks the kind pattern like the server", () => {
    for (const ok of ["mail.new", "github.*", "*", "host.alert*", "a-b_c.d"])
      expect(validKindPattern(ok)).toBe(true);
    for (const bad of ["", "Mail", "mail new", "mail/new", "x".repeat(65)])
      expect(validKindPattern(bad)).toBe(false);
  });

  it("turns unticked boxes into muted targets", () => {
    const all = ["webpush:1", "webpush:2", "bark"];
    expect(mutedFromChecked(all, new Set(["webpush:2", "bark"]))).toEqual([
      "webpush:1",
    ]);
    expect(mutedFromChecked(all, new Set(all))).toEqual([]);
    expect(mutedFromChecked(all, new Set())).toEqual(all);
  });

  it("compares target lists without order", () => {
    expect(sameTargets(["a", "b"], ["b", "a"])).toBe(true);
    expect(sameTargets(["a"], ["a", "b"])).toBe(false);
    expect(sameTargets([], [])).toBe(true);
  });
});
