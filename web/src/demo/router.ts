/*
 * 演示数据的小路由。handler 返回 undefined 表示不拦，交给真实服务端。
 */
export interface DemoRequest {
  method: string;
  path: string; // 去掉 /api/v1 的路径
  query: URLSearchParams;
  params: Record<string, string>;
  body: any; // eslint-disable-line @typescript-eslint/no-explicit-any
}

export type DemoHandler = (
  req: DemoRequest,
) => Response | undefined | Promise<Response | undefined>;

interface Route {
  method: string;
  pattern: RegExp;
  keys: string[];
  handler: DemoHandler;
}

const routes: Route[] = [];

/** route("GET", "/drive/items/:id", fn) */
export function route(method: string, path: string, handler: DemoHandler) {
  const keys: string[] = [];
  const pattern = new RegExp(
    "^" +
      path.replace(/:(\w+)/g, (_m, k: string) => {
        keys.push(k);
        return "([^/]+)";
      }) +
      "$",
  );
  routes.push({ method, pattern, keys, handler });
}

/** 找出所有匹配的处理函数，按登记顺序。前一个返回 undefined 时交给下一个。 */
export function findRoutes(method: string, path: string) {
  const out: { handler: DemoHandler; params: Record<string, string> }[] = [];
  for (const r of routes) {
    if (r.method !== method) continue;
    const m = r.pattern.exec(path);
    if (!m) continue;
    const params: Record<string, string> = {};
    r.keys.forEach((k, i) => (params[k] = decodeURIComponent(m[i + 1])));
    out.push({ handler: r.handler, params });
  }
  return out;
}

export const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

export const noContent = (status = 204) => new Response(null, { status });

export const fail = (status: number, code: string, message: string) =>
  json({ code, message }, status);

export const now = () => new Date().toISOString();

export const ago = (minutes: number) =>
  new Date(Date.now() - minutes * 60_000).toISOString();
