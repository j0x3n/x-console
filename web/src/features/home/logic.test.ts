import { describe, expect, it } from "vitest";
import {
  applyStateChange,
  displayName,
  formatState,
  groupByDomain,
  matches,
  move,
  rowActions,
  stateLabel,
  tapAction,
  type EntityState,
} from "./logic";

const s = (
  entityId: string,
  state: string,
  attributes: Record<string, unknown> = {},
): EntityState => ({
  entityId,
  state,
  attributes,
  lastChanged: "2026-09-27T00:00:00Z",
});

describe("tapAction", () => {
  it("toggles lights and switches", () => {
    expect(tapAction("light.a", s("light.a", "off"))).toEqual({
      kind: "toggle",
      domain: "light",
      service: "turn_on",
    });
    expect(tapAction("switch.b", s("switch.b", "on"))).toEqual({
      kind: "toggle",
      domain: "switch",
      service: "turn_off",
    });
  });
  it("runs scenes, scripts and buttons", () => {
    expect(tapAction("scene.movie")).toEqual({
      kind: "run",
      domain: "scene",
      service: "turn_on",
    });
    expect(tapAction("button.restart").kind).toBe("run");
  });
  it("does nothing for sensors and locks", () => {
    expect(tapAction("sensor.t", s("sensor.t", "21")).kind).toBe("none");
    expect(tapAction("lock.door", s("lock.door", "locked")).kind).toBe("none");
  });
});

describe("rowActions", () => {
  it("offers lock and cover controls", () => {
    expect(rowActions("lock.door", s("lock.door", "locked"))).toEqual([
      { label: "Unlock", domain: "lock", service: "unlock" },
    ]);
    expect(rowActions("cover.garage").map((a) => a.service)).toEqual([
      "open_cover",
      "close_cover",
    ]);
    expect(rowActions("light.a", s("light.a", "on"))[0].service).toBe(
      "turn_off",
    );
    expect(rowActions("sensor.t", s("sensor.t", "1"))).toEqual([]);
  });
  it("labels known states and keeps numbers", () => {
    expect(stateLabel(s("person.me", "not_home"))).toBe("Away");
    expect(stateLabel(s("sensor.h", "40", { unit_of_measurement: "%" }))).toBe(
      "40 %",
    );
  });
});

describe("display", () => {
  it("prefers alias, then friendly name", () => {
    const st = s("light.a", "on", { friendly_name: "Kitchen" });
    expect(displayName("light.a", st, "厨房")).toBe("厨房");
    expect(displayName("light.a", st)).toBe("Kitchen");
    expect(displayName("light.a")).toBe("light.a");
  });
  it("adds the unit to sensor values", () => {
    expect(formatState(s("sensor.t", "21.5", { unit_of_measurement: "°C" }))).toBe(
      "21.5 °C",
    );
    expect(formatState(s("light.a", "on"))).toBe("on");
  });
  it("searches id and name", () => {
    const st = s("sensor.t", "1", { friendly_name: "Living Room" });
    expect(matches(st, "living")).toBe(true);
    expect(matches(st, "sensor.")).toBe(true);
    expect(matches(st, "kitchen")).toBe(false);
    expect(matches(st, " ")).toBe(true);
  });
});

describe("groupByDomain", () => {
  it("puts common domains first and sorts by name", () => {
    const groups = groupByDomain([
      s("zone.home", "0"),
      s("sensor.b", "1", { friendly_name: "B" }),
      s("light.z", "on", { friendly_name: "Z" }),
      s("sensor.a", "1", { friendly_name: "A" }),
    ]);
    expect(groups.map((g) => g.domain)).toEqual(["light", "sensor", "zone"]);
    expect(groups[1].items.map((i) => i.entityId)).toEqual([
      "sensor.a",
      "sensor.b",
    ]);
  });
});

describe("list helpers", () => {
  it("replaces or appends a state", () => {
    const list = [s("light.a", "off"), s("light.b", "off")];
    expect(applyStateChange(list, s("light.b", "on"))[1].state).toBe("on");
    expect(applyStateChange(list, s("light.c", "on"))).toHaveLength(3);
  });
  it("moves items within bounds", () => {
    expect(move([1, 2, 3], 0, 1)).toEqual([2, 1, 3]);
    expect(move([1, 2, 3], 0, -1)).toEqual([1, 2, 3]);
    expect(move([1, 2, 3], 2, 1)).toEqual([1, 2, 3]);
  });
});
