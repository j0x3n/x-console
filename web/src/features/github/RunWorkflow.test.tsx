// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 页面之前替换。
// 记下每个请求的方法、地址和请求体，用来检查提交的内容。
const api = vi.hoisted(() => {
  const routes = new Map<string, unknown>();
  const calls: {
    method: string;
    path: string;
    query: string;
    body: unknown;
  }[] = [];
  const BaseRequest = globalThis.Request;
  globalThis.Request = class extends BaseRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(
        typeof input === "string" && input.startsWith("/")
          ? `http://localhost${input}`
          : input,
        init,
      );
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const req = input as Request;
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    const text = await req.text();
    calls.push({
      method: req.method,
      path,
      query: url.search,
      body: text ? JSON.parse(text) : undefined,
    });
    if (req.method === "POST" && !routes.has(`POST ${path}`))
      return new Response(null, { status: 204 });
    const value = routes.get(`${req.method} ${path}`) ?? routes.get(path);
    const failed =
      value && typeof value === "object" && "status" in value && "body" in value
        ? (value as { status: number; body: unknown })
        : null;
    return new Response(JSON.stringify(failed ? failed.body : (value ?? [])), {
      status: failed ? failed.status : 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return { routes, calls };
});

import "./i18n";
import GitHubPage from "./GitHubPage";
import RunWorkflowDialog, { initialInputs } from "./RunWorkflow";
import type { GitHubWorkflow } from "./api";

function wrap(node: React.ReactNode, path = "/github") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const deploy: GitHubWorkflow = {
  id: 42,
  name: "Deploy",
  path: ".github/workflows/deploy.yml",
  inputs: [
    {
      name: "env",
      description: "Where to",
      type: "choice",
      required: true,
      default: "staging",
      options: ["staging", "prod"],
    },
    {
      name: "dry",
      description: "Dry run",
      type: "boolean",
      required: false,
      default: "true",
      options: [],
    },
    {
      name: "note",
      description: "note",
      type: "string",
      required: false,
      options: [],
    },
    {
      name: "count",
      description: "How many",
      type: "number",
      required: false,
      default: "3",
      options: [],
    },
  ],
};

function watched(forge: "github" | "forgejo") {
  return [
    {
      connectionId: 1,
      connectionName: forge === "github" ? "GitHub" : "Forgejo",
      forge,
      repo: "acme/api",
      url: "https://github.com/acme/api",
      defaultBranch: "main",
      private: false,
      openPulls: 0,
      openIssues: 0,
      notifyCustom: false,
      notifyOff: false,
    },
  ];
}

const failedRun = {
  id: 7,
  connectionId: 1,
  forge: "github",
  repo: "acme/api",
  name: "CI",
  branch: "main",
  event: "push",
  status: "completed",
  conclusion: "failure",
  url: "https://x",
  defaultBranch: true,
  createdAt: "2026-10-18T10:00:00Z",
  updatedAt: "2026-10-18T10:00:00Z",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("initialInputs", () => {
  it("uses defaults, then false for checkboxes and the first option", () => {
    expect(
      initialInputs({
        ...deploy,
        inputs: [
          { ...deploy.inputs[0], default: undefined },
          { ...deploy.inputs[1], default: undefined },
          deploy.inputs[2],
          deploy.inputs[3],
        ],
      }),
    ).toEqual({ env: "staging", dry: "false", note: "", count: "3" });
  });
});

describe("RunWorkflowDialog", () => {
  it("B248: shows a control per input type and submits the values", async () => {
    api.routes.set("/github/repos/acme/api/workflows", { items: [deploy] });
    const onClose = vi.fn();
    wrap(
      <RunWorkflowDialog
        repo={{ repo: "acme/api", connectionId: 1 }}
        defaultBranch="main"
        onClose={onClose}
      />,
    );
    const env = (await screen.findByRole("combobox", {
      name: /env/,
    })) as HTMLSelectElement;
    expect(env.value).toBe("staging");
    const dry = screen.getByRole("checkbox", {
      name: "dry",
    }) as HTMLInputElement;
    expect(dry.checked).toBe(true);
    const count = screen.getByRole("spinbutton", {
      name: /count/,
    }) as HTMLInputElement;
    expect(count.value).toBe("3");
    expect(screen.getByRole("textbox", { name: "分支" })).toHaveProperty(
      "value",
      "main",
    );

    fireEvent.change(env, { target: { value: "prod" } });
    fireEvent.click(dry);
    fireEvent.change(screen.getByRole("textbox", { name: "note" }), {
      target: { value: "hello" },
    });
    fireEvent.click(screen.getByRole("button", { name: "运行" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const post = api.calls.find((c) => c.method === "POST")!;
    expect(post.path).toBe("/github/repos/acme/api/workflows/42/dispatches");
    expect(post.query).toBe("?connectionId=1");
    expect(post.body).toEqual({
      ref: "main",
      inputs: { env: "prod", dry: "false", note: "hello", count: "3" },
    });
  });

  it("B248: shows GitHub's message when the token lacks permission", async () => {
    api.routes.set("/github/repos/acme/api/workflows", { items: [deploy] });
    api.routes.set("POST /github/repos/acme/api/workflows/42/dispatches", {
      status: 502,
      body: {
        code: "git_service_error",
        message: "Resource not accessible by personal access token",
      },
    });
    const onClose = vi.fn();
    wrap(
      <RunWorkflowDialog
        repo={{ repo: "acme/api", connectionId: 1 }}
        defaultBranch="main"
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: /env/ });
    fireEvent.click(screen.getByRole("button", { name: "运行" }));
    expect(
      await screen.findByText(
        /Resource not accessible by personal access token/,
      ),
    ).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("GitHubPage runs view", () => {
  const status = { configured: true, syncing: false, repoCount: 1 };

  it("B248: shows Run workflow and rerun for GitHub repos", async () => {
    api.routes.set("/github/status", status);
    api.routes.set("/github/repos", watched("github"));
    api.routes.set("/github/runs", [failedRun]);
    wrap(<GitHubPage />, "/github?tab=runs&repo=1:acme/api");
    expect(
      await screen.findByRole("button", { name: "运行工作流" }),
    ).toBeTruthy();

    fireEvent.click((await screen.findByText("CI")).closest("button")!);
    fireEvent.click(
      await screen.findByRole("button", { name: "重新运行失败的任务" }),
    );
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) =>
            c.method === "POST" &&
            c.path === "/github/repos/acme/api/runs/7/rerun-failed",
        ),
      ).toBe(true),
    );
  });

  it("B248: hides both buttons for Forgejo repos", async () => {
    api.routes.set("/github/status", status);
    api.routes.set("/github/repos", watched("forgejo"));
    api.routes.set("/github/runs", [{ ...failedRun, forge: "forgejo" }]);
    wrap(<GitHubPage />, "/github?tab=runs&repo=1:acme/api");
    fireEvent.click((await screen.findByText("CI")).closest("button")!);
    await screen.findByText("Forgejo");
    expect(screen.queryByRole("button", { name: "运行工作流" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "重新运行失败的任务" }),
    ).toBeNull();
  });
});
