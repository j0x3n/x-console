import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError } from "./client";
import { reportError } from "../lib/errors";

/*
 * 全局兜底报错（B41）：
 * - 查询出错一律上报。接口还没上线（404、501）不报，页面会显示“还没上线”。
 *   meta: { silentError: true } 的查询只打印到控制台。
 * - 修改操作自己写了 onError 的，由它自己提示；没写的在这里上报。
 */
function silent(meta: Record<string, unknown> | undefined) {
  return meta?.silentError === true;
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error, query) => {
      if (
        error instanceof ApiError &&
        (error.status === 404 || error.status === 501)
      )
        return;
      reportError(error, { silent: silent(query.meta) });
    },
  }),
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      if (mutation.options.onError) return;
      reportError(error, { silent: silent(mutation.meta) });
    },
  }),
  defaultOptions: {
    queries: {
      // 数据靠服务端事件刷新，切回窗口时一分钟内不重复请求。
      staleTime: 60_000,
      refetchOnWindowFocus: true,
      retry: (count, error) =>
        !(error instanceof ApiError && error.status < 500) && count < 2,
    },
    mutations: { retry: false },
  },
});
