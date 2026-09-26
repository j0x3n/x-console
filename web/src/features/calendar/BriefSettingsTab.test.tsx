// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const server = vi.hoisted(() => {
  const state = {
    aiAvailable: false,
    puts: [] as Record<string, unknown>[],
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
});

describe("BriefSettingsTab", () => {
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
});
