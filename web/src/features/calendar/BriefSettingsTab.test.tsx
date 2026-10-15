// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const server = vi.hoisted(() => {
  const state = {
    aiAvailable: false,
    guessSleep: false,
    puts: [] as Record<string, unknown>[],
    places: [] as string[],
  };
  const NativeRequest = globalThis.Request;
  globalThis.Request = class extends NativeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(
        typeof input === "string" && input.startsWith("/")
          ? `http://localhost${input}`
          : input,
        init,
      );
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof NativeRequest ? input : new Request(String(input), init);
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    if (path === "/weather/places") {
      state.places.push(new URL(req.url).searchParams.get("q") ?? "");
      return new Response(
        JSON.stringify([
          {
            id: "101191002",
            name: "东海",
            region: "江苏",
            country: "中国",
            lat: 34.5225,
            lon: 118.7666,
          },
        ]),
        { headers: { "Content-Type": "application/json" } },
      );
    }
    if (path === "/me/settings")
      return new Response(
        JSON.stringify({
          workHosts: [],
          hosts: [],
          guessSleep: state.guessSleep,
          haHome: "",
          haSleep: "",
        }),
        { headers: { "Content-Type": "application/json" } },
      );
    if (path === "/me/report")
      return new Response(JSON.stringify({ enabled: false, reportUrl: "" }), {
        headers: { "Content-Type": "application/json" },
      });
    const view = {
      enabled: true,
      time: "08:00",
      channels: [],
      sections: ["weather", "calendar"],
      aiPolish: false,
      weatherApiBase: "https://api.open-meteo.com",
      availableChannels: ["bark", "telegram"],
      aiAvailable: state.aiAvailable,
    };
    if (path === "/briefs/settings" && req.method === "PUT") {
      const body = await req.clone().json();
      state.puts.push(body);
      return new Response(JSON.stringify({ ...view, ...body }), {
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response(JSON.stringify(view), {
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return state;
});

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { LanguageContext } from "../../contexts/LanguageContext";
import BriefSettingsTab from "./BriefSettingsTab";

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <LanguageContext.Provider value="en">
        <MemoryRouter>
          <BriefSettingsTab />
        </MemoryRouter>
      </LanguageContext.Provider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  server.puts.length = 0;
  server.places.length = 0;
  server.guessSleep = false;
  vi.unstubAllGlobals();
});

describe("BriefSettingsTab", () => {
  it("reverse-geocodes browser coordinates through QWeather before saving", async () => {
    vi.stubGlobal("navigator", {
      ...navigator,
      geolocation: {
        getCurrentPosition: (success: PositionCallback) =>
          success({
            coords: { latitude: 34.54, longitude: 118.75 },
          } as GeolocationPosition),
      },
    });
    renderTab();
    fireEvent.click(await screen.findByText("Use my location"));
    await waitFor(() =>
      expect(screen.getByPlaceholderText("31.23")).toHaveProperty(
        "value",
        "34.5225",
      ),
    );
    expect(server.places).toEqual(["118.75,34.54"]);
    fireEvent.click(screen.getByText("Save"));
    await waitFor(() => expect(server.puts).toHaveLength(1));
    expect(server.puts[0].location).toEqual({
      id: "101191002",
      name: "东海",
      lat: 34.5225,
      lon: 118.7666,
    });
    expect(screen.queryByText("Open-Meteo address")).toBeNull();
  });
  it("hides the AI summary switch until the AI module offers it", async () => {
    server.aiAvailable = false;
    renderTab();
    await screen.findByText("Send the brief every day");
    expect(screen.queryByText("Start with a short AI summary")).toBeNull();

    fireEvent.click(screen.getByLabelText("Telegram"));
    fireEvent.change(screen.getByPlaceholderText("31.23"), {
      target: { value: "39.9" },
    });
    fireEvent.click(screen.getByText("Save"));
    await screen.findByText("Enter both latitude and longitude.");
    fireEvent.change(screen.getByPlaceholderText("121.47"), {
      target: { value: "116.4" },
    });
    fireEvent.click(screen.getByText("Save"));
    await waitFor(() => expect(server.puts).toHaveLength(1));
    expect(server.puts[0]).toMatchObject({
      channels: ["telegram"],
      location: { lat: 39.9, lon: 116.4 },
    });
    expect(server.puts[0]).not.toHaveProperty("aiPolish");
  });

  it("shows the switch when AI is available", async () => {
    server.aiAvailable = true;
    renderTab();
    expect(
      await screen.findByText("Start with a short AI summary"),
    ).toBeTruthy();
  });

  it("sends after waking up and says when there is no wake-up signal", async () => {
    renderTab();
    fireEvent.click(
      await screen.findByLabelText("After I wake up, at the latest"),
    );
    await screen.findByText(/No wake-up signal yet/);
    fireEvent.click(screen.getByText("Save"));
    await waitFor(() => expect(server.puts).toHaveLength(1));
    expect(server.puts[0]).toMatchObject({ sendMode: "wake", time: "08:00" });
  });

  it("has no hint when sleep is guessed", async () => {
    server.guessSleep = true;
    renderTab();
    fireEvent.click(
      await screen.findByLabelText("After I wake up, at the latest"),
    );
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByText(/No wake-up signal yet/)).toBeNull();
    expect(screen.getByLabelText("At a fixed time")).toHaveProperty(
      "checked",
      false,
    );
  });
});
