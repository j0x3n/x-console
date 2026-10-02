import { describe, expect, it } from "vitest";
import { flagEmoji, moveId, sortByIds } from "./api";
import { infoInput, infoDraft, splitTags } from "./components/HostInfo";
import { sortAddresses } from "./components/AddressList";

describe("B82 host helpers", () => {
  it("moves an id before or after the target", () => {
    const ids = ["a", "b", "c", "d"];
    expect(moveId(ids, "c", "a")).toEqual(["c", "a", "b", "d"]);
    expect(moveId(ids, "a", "c")).toEqual(["b", "c", "a", "d"]);
    expect(moveId(ids, "b", "b")).toEqual(ids);
    expect(moveId(ids, "x", "a")).toEqual(ids);
  });

  it("sorts by the given order and keeps unknown ones last", () => {
    const list = [{ id: "a" }, { id: "b" }, { id: "c" }];
    expect(sortByIds(list, ["c", "a"]).map((h) => h.id)).toEqual([
      "c",
      "a",
      "b",
    ]);
  });

  it("turns a country code into a flag", () => {
    expect(flagEmoji("jp")).toBe("🇯🇵");
    expect(flagEmoji("XYZ")).toBe("");
  });

  it("splits tags and builds the info input", () => {
    expect(splitTags("生产, 香港，生产、 ")).toEqual(["生产", "香港"]);
    const d = {
      ...infoDraft(),
      ownership: "client" as const,
      client: " A ",
      tags: "x",
    };
    expect(infoInput(d)).toEqual({
      ownership: "client",
      client: "A",
      username: "",
      note: "",
      tags: ["x"],
    });
    expect(infoInput({ ...d, password: "p" }).password).toBe("p");
    expect(infoInput({ ...d, clearPassword: true }).clearPassword).toBe(true);
  });

  it("puts public addresses and IPv4 first", () => {
    const list = sortAddresses([
      { ip: "10.0.0.1", family: "v4", public: false },
      { ip: "2001:db8::1", family: "v6", public: true },
      { ip: "1.2.3.4", family: "v4", public: true },
    ]);
    expect(list.map((a) => a.ip)).toEqual([
      "1.2.3.4",
      "2001:db8::1",
      "10.0.0.1",
    ]);
  });
});
