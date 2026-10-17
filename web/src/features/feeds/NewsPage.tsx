import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { ExternalLink, Newspaper, Send } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, Loading } from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import {
  IMPORTANT_MIN,
  useFeedsSettings,
  useNews,
  useNewsFacets,
  type FeedsNewsItem,
  type NewsFilter,
} from "./api";
import { FeedsError, NotConfigured } from "./FeedsState";
import NewsDetail from "./NewsDetail";
import SaveLink from "./SaveLink";
import { useDebounced, useParamSetter } from "./params";
import "./i18n";
import "./feeds.css";

const DAY_OPTIONS = [
  { value: 0, label: "All time" },
  { value: 1, label: "Last 24 hours" },
  { value: 7, label: "Last 7 days" },
  { value: 30, label: "Last 30 days" },
] as const;

/** 从地址栏读筛选条件。 */
export function readNewsFilter(params: URLSearchParams): NewsFilter {
  const days = Number(params.get("days") ?? "0");
  return {
    q: params.get("q") ?? "",
    tag: params.get("tag") ?? "",
    category: params.get("category") ?? "",
    days: DAY_OPTIONS.some((o) => o.value === days) ? days : 0,
    important: params.get("important") === "1",
    pushed: params.get("pushed") === "1",
  };
}

/** 资讯里的新闻（B241）：ai-news 抓到的新闻，按抓取时间倒序。 */
export default function NewsPage() {
  const t = useT();
  const language = useLanguage();
  const [params] = useSearchParams();
  const set = useParamSetter();
  const filter = readNewsFilter(params);
  const [q, setQ] = useState(filter.q);
  const debounced = useDebounced(q.trim());
  useEffect(() => {
    if (debounced !== filter.q) set({ q: debounced || null });
    // 只在输入停下后写回地址栏
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);
  const openParam = Number(params.get("item"));
  const openId =
    Number.isInteger(openParam) && openParam > 0 ? openParam : null;

  const settings = useFeedsSettings();
  const ready = settings.data?.configured === true;
  const list = useNews(filter, ready);
  const facets = useNewsFacets(ready);

  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered =
    filter.q !== "" ||
    filter.tag !== "" ||
    filter.category !== "" ||
    filter.days !== 0 ||
    filter.important ||
    filter.pushed;
  const latest = items[0]?.fetchedAt;

  let body;
  if (settings.isPending) body = <Loading />;
  else if (settings.isError || !ready)
    body = settings.isError ? (
      <FeedsError error={settings.error} onRetry={() => settings.refetch()} />
    ) : (
      <NotConfigured />
    );
  else if (list.isPending) body = <Loading />;
  else if (list.isError)
    body = <FeedsError error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0)
    body = (
      <EmptyState
        title={filtered ? t("No news matches") : t("No news yet")}
        icon={<Newspaper size={28} />}
      >
        {filtered && (
          <button
            className="xc-btn small"
            onClick={() => {
              setQ("");
              set({
                q: null,
                tag: null,
                category: null,
                days: null,
                important: null,
                pushed: null,
              });
            }}
          >
            {t("Clear filters")}
          </button>
        )}
      </EmptyState>
    );
  else
    body = (
      <div className="xc-card feeds-list">
        {items.map((item) => (
          <NewsRow
            key={item.id}
            item={item}
            onOpen={() => set({ item: String(item.id) }, false)}
            onTag={(tag) => set({ tag })}
          />
        ))}
        {list.hasNextPage && (
          <div className="feeds-more">
            <button
              className="xc-btn small ghost"
              disabled={list.isFetchingNextPage}
              onClick={() => list.fetchNextPage()}
            >
              {t("Load more")}
            </button>
          </div>
        )}
      </div>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("News")}
        subtitle={latest ? relativeTime(latest, language) : undefined}
      />
      {ready && (
        <Toolbar
          start={
            <div className="feeds-filters">
              <select
                className="xc-select"
                aria-label={t("All categories")}
                value={filter.category}
                onChange={(e) => set({ category: e.target.value || null })}
              >
                <option value="">{t("All categories")}</option>
                {facets.data?.categories.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
                {filter.category &&
                  !facets.data?.categories.includes(filter.category) && (
                    <option value={filter.category}>{filter.category}</option>
                  )}
              </select>
              <select
                className="xc-select"
                aria-label={t("All time")}
                value={filter.days}
                onChange={(e) =>
                  set({ days: e.target.value === "0" ? null : e.target.value })
                }
              >
                {DAY_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {t(o.label)}
                  </option>
                ))}
              </select>
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={filter.important}
                  onChange={(e) =>
                    set({ important: e.target.checked ? "1" : null })
                  }
                />
                {t("Important only")}
              </label>
              <label className="xc-check">
                <input
                  type="checkbox"
                  checked={filter.pushed}
                  onChange={(e) =>
                    set({ pushed: e.target.checked ? "1" : null })
                  }
                />
                {t("Pushed only")}
              </label>
              {filter.tag && (
                <button
                  className="xc-badge accent feeds-tag-filter"
                  title={t("Clear")}
                  onClick={() => set({ tag: null })}
                >
                  {filter.tag} ×
                </button>
              )}
            </div>
          }
          end={
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search news")}
              clearLabel={t("Clear")}
            />
          }
        />
      )}
      {body}
      <NewsDetail id={openId} onClose={() => set({ item: null })} />
    </div>
  );
}

function NewsRow({
  item,
  onOpen,
  onTag,
}: {
  item: FeedsNewsItem;
  onOpen: () => void;
  onTag: (tag: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const title = item.titleCn || item.title;
  const summary = item.summaryCn || item.summary;
  const when = item.fetchedAt ?? item.publishedAt;
  return (
    <div className="feeds-row">
      <div className="feeds-main">
        <button className="feeds-open" onClick={onOpen}>
          <strong className="feeds-title">{title}</strong>
          <small>
            {[item.source, item.category].filter(Boolean).join(" · ")}
            {when && ` · ${relativeTime(when, language)}`}
          </small>
          {summary && <span className="feeds-clip">{summary}</span>}
        </button>
        {(item.tags.length > 0 || item.importance != null || item.pushed) && (
          <span className="feeds-tagline">
            {item.importance != null && (
              <span
                className={`xc-badge${item.importance >= IMPORTANT_MIN ? " warn" : ""}`}
                title={t("Importance")}
              >
                {t("Importance")} {item.importance}
              </span>
            )}
            {item.pushed && (
              <span className="xc-badge info" title={t("Pushed to Telegram")}>
                <Send size={11} /> Telegram
              </span>
            )}
            {item.tags.slice(0, 8).map((tag) => (
              <button key={tag} className="xc-badge" onClick={() => onTag(tag)}>
                {tag}
              </button>
            ))}
          </span>
        )}
      </div>
      <div className="feeds-side">
        <a
          className="xc-btn ghost small icon"
          href={item.url}
          target="_blank"
          rel="noopener noreferrer"
          title={t("Open the original page")}
          aria-label={`${t("Open the original page")} ${title}`}
        >
          <ExternalLink size={15} />
        </a>
        <SaveLink url={item.url} label={title} compact />
      </div>
    </div>
  );
}
