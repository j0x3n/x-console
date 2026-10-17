import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ApiError, createApi, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import type { components, paths } from "../../api/gen/feeds";

/* 资讯（B241）：面板经过虚拟机上的代理，去读 ai-news 的新闻和 x-assistant 的推文。数据不存在面板里。 */
export const feedsApi = createApi<paths>();

type S = components["schemas"];
export type FeedsSettings = S["FeedsSettings"];
export type FeedsSettingsInput = S["FeedsSettingsInput"];
export type FeedsHealth = S["FeedsHealth"];
export type FeedsNewsItem = S["FeedsNewsItem"];
export type FeedsNewsDetail = S["FeedsNewsDetail"];
export type FeedsTweet = S["FeedsTweet"];

/** 新闻的筛选条件，都写在地址栏里。 */
export interface NewsFilter {
  q: string;
  tag: string;
  category: string;
  /** 1 是最近 24 小时，0 是全部。 */
  days: number;
  important: boolean;
  pushed: boolean;
}

export interface TweetFilter {
  q: string;
  author: string;
}

export const feedsKeys = {
  all: ["feeds"] as const,
  settings: ["feeds", "settings"] as const,
  news: (f: NewsFilter) => ["feeds", "news", f] as const,
  facets: ["feeds", "facets"] as const,
  item: (id: number) => ["feeds", "item", id] as const,
  tweets: (f: TweetFilter) => ["feeds", "tweets", f] as const,
};

/** 后端约定的错误码，页面按它给出不同的提示。 */
export function feedsErrorCode(error: unknown): string {
  return error instanceof ApiError ? error.code : "";
}

/** 只看重要的：ai-news 的重要度 4 分以上。 */
export const IMPORTANT_MIN = 4;

const PAGE = 50;

export function useFeedsSettings() {
  return useQuery({
    queryKey: feedsKeys.settings,
    queryFn: () => unwrap(feedsApi.GET("/feeds/settings")),
  });
}

export function useSaveFeedsSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: FeedsSettingsInput) =>
      withElevation(() => unwrap(feedsApi.PUT("/feeds/settings", { body }))),
    onSuccess: () => qc.invalidateQueries({ queryKey: feedsKeys.all }),
  });
}

export function useTestFeeds() {
  return useMutation({
    mutationFn: () => unwrap(feedsApi.POST("/feeds/settings/test")),
  });
}

// 虚拟机关机、接口没起来时重试也没用，直接显示原因。
const noRetry = { retry: false, meta: { silentError: true } } as const;

export function useNews(filter: NewsFilter, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: feedsKeys.news(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        feedsApi.GET("/feeds/news", {
          params: {
            query: {
              q: filter.q || undefined,
              tag: filter.tag || undefined,
              category: filter.category || undefined,
              days: filter.days || undefined,
              minImportance: filter.important ? IMPORTANT_MIN : undefined,
              pushed: filter.pushed || undefined,
              before: pageParam || undefined,
              limit: PAGE,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.next ?? undefined,
    placeholderData: (prev) => prev,
    enabled,
    ...noRetry,
  });
}

export function useNewsFacets(enabled: boolean) {
  return useQuery({
    queryKey: feedsKeys.facets,
    queryFn: () => unwrap(feedsApi.GET("/feeds/news/facets")),
    staleTime: 5 * 60_000,
    enabled,
    ...noRetry,
  });
}

export function useNewsItem(id: number | null) {
  return useQuery({
    queryKey: feedsKeys.item(id ?? 0),
    queryFn: () =>
      unwrap(
        feedsApi.GET("/feeds/news/{id}", {
          params: { path: { id: id ?? 0 } },
        }),
      ),
    enabled: id != null,
    ...noRetry,
  });
}

export function useTweets(filter: TweetFilter, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: feedsKeys.tweets(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        feedsApi.GET("/feeds/tweets", {
          params: {
            query: {
              q: filter.q || undefined,
              author: filter.author || undefined,
              before: pageParam || undefined,
              limit: PAGE,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.next ?? undefined,
    placeholderData: (prev) => prev,
    enabled,
    ...noRetry,
  });
}
