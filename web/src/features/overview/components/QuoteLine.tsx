import { useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { unwrap } from "../../../api/client";
import { usePreferencesStore } from "../../../stores/preferences-store";
import { notesApi } from "../../notes/api";
import { pickQuote, QUOTE_TAG, quoteText } from "../quote";

/** 本地日期，比如 2026-10-02。 */
function today() {
  const d = new Date();
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/**
 * B89：问候语后面的每日一句。来源是带“名言”标签的便签，
 * 显示方式在 笔记 → 便签 的“每日一句”里设。点它打开那条便签。
 */
export default function QuoteLine() {
  const mode = usePreferencesStore((s) => s.quoteMode);
  // “每次打开随机”：这一页打开时取一次随机数
  const [random] = useState(() => Math.random());
  const quotes = useQuery({
    queryKey: ["notes", "quotes"],
    queryFn: () =>
      unwrap(
        notesApi.GET("/notes", {
          params: { query: { tag: QUOTE_TAG, kind: "memo", limit: 100 } },
        }),
      ),
    enabled: mode !== "off",
    staleTime: 5 * 60_000,
    retry: false,
    meta: { silentError: true },
  });
  const list = Array.isArray(quotes.data?.items) ? quotes.data.items : [];
  const quote = pickQuote(list, mode, today(), random);
  if (!quote) return null;
  return (
    <Link
      className="today-quote"
      to={`/notes?view=memos&tag=${encodeURIComponent(QUOTE_TAG)}`}
      title={quoteText(quote)}
    >
      {quoteText(quote)}
    </Link>
  );
}
