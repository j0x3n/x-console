import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/cards";

/* 复习卡片（B130）：从笔记和稍后阅读里选一段文字做成一问一答，按 FSRS 排复习。 */
export const cardsApi = createApi<paths>();

type S = components["schemas"];
export type Card = S["Card"];
export type CardInput = S["CardInput"];
export type CardPatch = S["CardPatch"];
export type CardState = S["CardState"];
export type CardSummary = S["CardSummary"];
export type CardDeck = S["CardDeck"];
export type CardQueueItem = S["CardQueueItem"];
export type CardIntervals = S["CardIntervals"];
export type CardDraft = S["CardDraft"];
export type CardSettings = S["CardSettings"];
export type CardSettingsInput = S["CardSettingsInput"];
export type CardReviewResult = S["CardReviewResult"];
export type CardStyle = "qa" | "cloze";

export interface CardFilter {
  q: string;
  deck: string;
  state: CardState | "";
  suspended: boolean | null;
}

export const cardKeys = {
  all: ["cards"] as const,
  summary: ["cards", "summary"] as const,
  decks: ["cards", "decks"] as const,
  list: (f: CardFilter) => ["cards", "list", f] as const,
  settings: ["cards", "settings"] as const,
};

// 服务端新建、修改、删除、评分都会发 card.* 事件
invalidateOn("card.", cardKeys.all);

/** 今天还要复习多少张。今日页、左栏和复习页的概要共用它。 */
export function useCardSummary() {
  return useQuery({
    queryKey: cardKeys.summary,
    queryFn: () => unwrap(cardsApi.GET("/cards/summary")),
    retry: false,
  });
}

export function useCardDecks() {
  return useQuery({
    queryKey: cardKeys.decks,
    queryFn: () => unwrap(cardsApi.GET("/cards/decks")),
    retry: false,
  });
}

/** 卡片列表，滚到底再取下一页。 */
export function useCards(filter: CardFilter) {
  return useInfiniteQuery({
    queryKey: cardKeys.list(filter),
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      unwrap(
        cardsApi.GET("/cards", {
          params: {
            query: {
              q: filter.q || undefined,
              deck: filter.deck || undefined,
              state: filter.state || undefined,
              suspended: filter.suspended ?? undefined,
              cursor: pageParam || undefined,
              limit: 50,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    retry: false,
  });
}

/** 取今天的复习队列。开始复习时调一次，之后在页面里一张张往下走。 */
export function fetchQueue(deck: string) {
  return unwrap(
    cardsApi.GET("/cards/queue", {
      params: { query: { deck: deck || undefined, limit: 200 } },
    }),
  );
}

export function useCreateCard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CardInput) => unwrap(cardsApi.POST("/cards", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: cardKeys.all }),
  });
}

export function useUpdateCard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: number; body: CardPatch }) =>
      unwrap(
        cardsApi.PATCH("/cards/{cardId}", {
          params: { path: { cardId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: cardKeys.all }),
  });
}

export function useDeleteCard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        cardsApi.DELETE("/cards/{cardId}", {
          params: { path: { cardId: id } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: cardKeys.all }),
  });
}

/** 评一次分。reps 是卡片这次看到的次数，别处已经评过时服务端会拒绝。 */
export function useReviewCard() {
  return useMutation({
    mutationFn: (v: { id: number; rating: number; reps: number }) =>
      unwrap(
        cardsApi.POST("/cards/{cardId}/review", {
          params: { path: { cardId: v.id } },
          body: { rating: v.rating, reps: v.reps },
        }),
      ),
  });
}

/** 让 AI 写一张卡片的草稿，不入库。 */
export function useGenerateCard() {
  return useMutation({
    mutationFn: (body: {
      text: string;
      style: CardStyle;
      sourceTitle?: string;
    }) => unwrap(cardsApi.POST("/cards/generate", { body })),
  });
}

export function useCardSettings() {
  return useQuery({
    queryKey: cardKeys.settings,
    queryFn: () => unwrap(cardsApi.GET("/cards/settings")),
    retry: false,
  });
}

export function usePutCardSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CardSettingsInput) =>
      unwrap(cardsApi.PUT("/cards/settings", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: cardKeys.all }),
  });
}
