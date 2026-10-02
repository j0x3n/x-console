import { describe, expect, it } from "vitest";
import { CloudDrizzle, CloudMoon, CloudRain, Moon, Sun } from "lucide-react";
import { kindFromQWeather, kindFromWmo, weatherIcon } from "./weatherIcon";

describe("B90 weather icons", () => {
  it("maps WMO codes", () => {
    expect(kindFromWmo(53)).toBe("drizzle");
    expect(kindFromWmo(2)).toBe("partly");
    expect(kindFromWmo(81)).toBe("rain");
    expect(kindFromWmo(96)).toBe("hail");
  });
  it("maps QWeather codes", () => {
    expect(kindFromQWeather("309")).toBe("drizzle");
    expect(kindFromQWeather("305")).toBe("rain");
    expect(kindFromQWeather("151")).toBe("partly");
    expect(kindFromQWeather("404")).toBe("sleet");
    expect(kindFromQWeather("502")).toBe("haze");
  });
  it("prefers the QWeather icon and knows day and night", () => {
    expect(weatherIcon({ weatherCode: 2, icon: "309" })).toBe(CloudDrizzle);
    expect(weatherIcon({ weatherCode: 53 })).toBe(CloudDrizzle);
    expect(weatherIcon({ weatherCode: 61, icon: "999" })).toBe(CloudRain);
    expect(weatherIcon({ weatherCode: 0, isDay: true })).toBe(Sun);
    expect(weatherIcon({ weatherCode: 0, isDay: false })).toBe(Moon);
    expect(weatherIcon({ weatherCode: 1, isDay: false })).toBe(CloudMoon);
  });
});
