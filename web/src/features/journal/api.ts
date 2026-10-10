import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/journal";

/* 每日时间线和日记（B118）。 */
export const journalApi = createApi<paths>();

type S = components["schemas"];
export type JournalDay = S["JournalDay"];
export type JournalItem = S["JournalItem"];
export type JournalKind = S["JournalKind"];
export type JournalCount = S["JournalCount"];
export type JournalHit = S["JournalHit"];
export type JournalRecentDay = S["JournalRecentDay"];

export const journalKeys = {
  all: ["journal"] as const,
  day: (day: string) => ["journal", "day", day] as const,
  recent: (days: number) => ["journal", "recent", days] as const,
  search: (q: string) => ["journal", "search", q] as const,
};

// 日记改了、时间线有新内容时，服务端发事件
invalidateOn("journal.", journalKeys.all);

export function useJournalDay(day: string | null) {
  return useQuery({
    queryKey: journalKeys.day(day ?? ""),
    enabled: !!day,
    queryFn: () =>
      unwrap(
        journalApi.GET("/journal/days/{day}", {
          params: { path: { day: day ?? "" } },
        }),
      ),
    retry: false,
  });
}

/** 最近几天。第一项总是服务器上的“今天”，页面用它当默认日期。 */
export function useJournalRecent(days = 14) {
  return useQuery({
    queryKey: journalKeys.recent(days),
    queryFn: () =>
      unwrap(
        journalApi.GET("/journal/recent", { params: { query: { days } } }),
      ),
    retry: false,
  });
}

export function useJournalSearch(q: string) {
  return useQuery({
    queryKey: journalKeys.search(q),
    enabled: q !== "",
    queryFn: () =>
      unwrap(journalApi.GET("/journal/search", { params: { query: { q } } })),
    retry: false,
  });
}

export function usePutDiary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { day: string; body: string }) =>
      unwrap(
        journalApi.PUT("/journal/days/{day}/diary", {
          params: { path: { day: v.day } },
          body: { body: v.body },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: journalKeys.all }),
  });
}
