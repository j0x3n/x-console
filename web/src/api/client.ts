import createClient, { type Middleware } from "openapi-fetch";

/*
 * 每个模块用 createApi<paths>() 建自己的客户端，paths 来自 api/gen/<模块>.ts。
 * 请求统一带 CSRF 头；出错时抛 ApiError，交给 TanStack Query 处理。
 */

export const API_BASE = "/api/v1";

/** 出错请求的上下文，复制报错时用（B41）。 */
export interface ApiRequestInfo {
  method: string;
  path: string;
  /** 响应正文，最多 4 KB。 */
  body?: string;
  requestId?: string;
  at: number;
}

export class ApiError extends Error {
  status: number;
  code: string;
  details?: Record<string, unknown>;
  request?: ApiRequestInfo;
  constructor(
    status: number,
    code: string,
    message: string,
    details?: Record<string, unknown>,
  ) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

const listeners = new Set<() => void>();

/** 会话失效时回调，AuthGate 用它刷新登录状态。 */
export function onUnauthorized(fn: () => void) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** 最近的接口报错，新的在前，最多 20 条。lib/errors 用它给页面的报错提示补上详情。 */
export const recentApiErrors: ApiError[] = [];

async function checkResponse(
  response: Response,
  method = "GET",
): Promise<Response> {
  if (response.ok) return response;
  let body: {
    code?: string;
    message?: string;
    details?: Record<string, unknown>;
    requestId?: string;
  } = {};
  let text = "";
  try {
    text = await response.clone().text();
    body = JSON.parse(text);
  } catch {
    /* 非 JSON 错误体 */
  }
  if (response.status === 401 && body.code === "unauthorized")
    listeners.forEach((fn) => fn());
  const error = new ApiError(
    response.status,
    body.code ?? "http_error",
    body.message ?? response.statusText,
    body.details,
  );
  let path = response.url;
  try {
    const url = new URL(response.url);
    path = url.pathname + url.search;
  } catch {
    /* 测试里的假响应没有地址 */
  }
  error.request = {
    method,
    path,
    body: text.slice(0, 4096),
    requestId:
      body.requestId ?? response.headers?.get?.("X-Request-Id") ?? undefined,
    at: Date.now(),
  };
  recentApiErrors.unshift(error);
  recentApiErrors.length = Math.min(recentApiErrors.length, 20);
  throw error;
}

const errorMiddleware: Middleware = {
  onResponse: ({ response, request }) =>
    checkResponse(response, request.method),
};

export function createApi<Paths extends {}>() {
  const client = createClient<Paths>({
    baseUrl: API_BASE,
    credentials: "same-origin",
    headers: { "X-Requested-With": "x-console" },
  });
  client.use(errorMiddleware);
  return client;
}

/**
 * 取出 openapi-fetch 结果里的 data。错误已经由中间件抛出，
 * 这里只处理 204 之类没有返回体的情况。
 */
export async function unwrap<T>(
  request: Promise<{ data?: T; error?: unknown }>,
): Promise<T> {
  const { data } = await request;
  return data as T;
}

/** 非 openapi 场景（上传、下载）用的 fetch，行为和 createApi 一致。 */
export async function apiFetch(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("X-Requested-With", "x-console");
  return checkResponse(
    await fetch(API_BASE + path, {
      ...init,
      headers,
      credentials: "same-origin",
    }),
    init.method ?? "GET",
  );
}

/** WebSocket 地址，例如 wsUrl("/hosts/abc/terminal")。 */
export function wsUrl(path: string) {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}${API_BASE}${path}`;
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error) return error.message;
  return String(error);
}

/** 接口在契约里有、后端还没做（404 或 501）。页面显示“还没上线”。 */
export function isNotLive(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  );
}
