// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { uploadUrl, useUploads } from "./upload";

class RequestStub {
  static requests: RequestStub[] = [];
  upload = { onprogress: (_event: { loaded: number }) => {} };
  onload = () => {};
  onerror = () => {};
  status = 200;
  statusText = "";
  responseText = "";
  headers: Record<string, string> = {};
  method = "";
  url = "";
  body: unknown;
  withCredentials = false;
  constructor() {
    RequestStub.requests.push(this);
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(key: string, value: string) {
    this.headers[key] = value;
  }
  send(body: unknown) {
    this.body = body;
  }
}
afterEach(async () => {
  for (const request of RequestStub.requests) request.onload();
  await vi.waitFor(() =>
    expect(
      useUploads
        .getState()
        .tasks.every(
          (task) => task.state === "done" || task.state === "failed",
        ),
    ).toBe(true),
  );
  useUploads.setState({ tasks: [] });
  RequestStub.requests = [];
  vi.unstubAllGlobals();
});
function start() {
  vi.stubGlobal("XMLHttpRequest", RequestStub);
}

describe("外部网盘上传队列", () => {
  it("保留内置云盘的 multipart 上传，外部网盘使用 PUT 原始文件", async () => {
    start();
    const file = new File(["hello"], "文件 #1.txt");
    const done = vi.fn();
    useUploads
      .getState()
      .add([file], { remoteId: 7, parent: "/目录/?x=1" }, done);
    const request = RequestStub.requests[0];
    expect(request.method).toBe("PUT");
    const url = new URL(request.url, "http://localhost");
    expect(url.pathname).toBe("/api/v1/storage/remotes/7/content");
    expect(url.searchParams.get("parent")).toBe("/目录/?x=1");
    expect(url.searchParams.get("name")).toBe(file.name);
    expect(request.body).toBe(file);
    expect(request.headers["X-Requested-With"]).toBe("x-console");
    expect(request.headers["Content-Type"]).toBe("application/octet-stream");
    request.upload.onprogress({ loaded: 3 });
    expect(useUploads.getState().tasks[0].loaded).toBe(3);
    request.onload();
    await vi.waitFor(() => expect(done).toHaveBeenCalledTimes(1));
    expect(useUploads.getState().tasks[0].state).toBe("done");
    useUploads.getState().add([file], { parent: 12, hidden: true }, vi.fn());
    expect(RequestStub.requests[1].method).toBe("POST");
    expect(RequestStub.requests[1].body).toBeInstanceOf(FormData);
    expect(uploadUrl({ parent: 12, hidden: true })).toBe(
      "/api/v1/drive/upload?parent=12&hidden=true",
    );
  });

  it("最多并行 3 个，后续文件保持原目标，失败保留后端提示", async () => {
    start();
    useUploads.getState().add(
      Array.from({ length: 4 }, (_, i) => new File(["x"], `${i}.txt`)),
      { remoteId: 8, parent: "original" },
      vi.fn(),
    );
    expect(RequestStub.requests).toHaveLength(3);
    expect(useUploads.getState().tasks[3].state).toBe("queued");
    RequestStub.requests[0].status = 403;
    RequestStub.requests[0].responseText = JSON.stringify({
      message: "网盘不允许上传",
    });
    RequestStub.requests[0].onload();
    await vi.waitFor(() => expect(RequestStub.requests).toHaveLength(4));
    expect(
      new URL(RequestStub.requests[3].url, "http://localhost").searchParams.get(
        "parent",
      ),
    ).toBe("original");
    expect(useUploads.getState().tasks[0]).toMatchObject({
      state: "failed",
      error: "网盘不允许上传",
    });
  });
});
