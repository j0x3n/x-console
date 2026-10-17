import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { ExternalLink, MessageSquareText } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, Loading } from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useFeedsSettings, useTweets, type FeedsTweet } from "./api";
import { FeedsError, NotConfigured } from "./FeedsState";
import SaveLink from "./SaveLink";
import { useDebounced, useParamSetter } from "./params";
import "./i18n";
import "./feeds.css";

/** 资讯里的推文（B241）：x-assistant 从 X 列表里抓到的推文，按发布时间倒序。 */
export default function TweetsPage() {
  const t = useT();
  const language = useLanguage();
  const [params] = useSearchParams();
  const set = useParamSetter();
  const urlQ = params.get("q") ?? "";
  const urlAuthor = params.get("author") ?? "";
  const [q, setQ] = useState(urlQ);
  const [author, setAuthor] = useState(urlAuthor);
  const dq = useDebounced(q.trim());
  const da = useDebounced(author.trim());
  useEffect(() => {
    if (dq !== urlQ || da !== urlAuthor)
      set({ q: dq || null, author: da || null });
    // 只在输入停下后写回地址栏
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dq, da]);

  const settings = useFeedsSettings();
  const ready = settings.data?.configured === true;
  const list = useTweets({ q: urlQ, author: urlAuthor }, ready);
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered = urlQ !== "" || urlAuthor !== "";
  const latest = items[0]?.time;

  let body;
  if (settings.isPending) body = <Loading />;
  else if (settings.isError)
    body = (
      <FeedsError error={settings.error} onRetry={() => settings.refetch()} />
    );
  else if (!ready) body = <NotConfigured />;
  else if (list.isPending) body = <Loading />;
  else if (list.isError)
    body = <FeedsError error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0)
    body = (
      <EmptyState
        title={filtered ? t("No tweets match") : t("No tweets yet")}
        icon={<MessageSquareText size={28} />}
      >
        {filtered && (
          <button
            className="xc-btn small"
            onClick={() => {
              setQ("");
              setAuthor("");
              set({ q: null, author: null });
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
        {items.map((tweet) => (
          <TweetRow
            key={tweet.id}
            tweet={tweet}
            onAuthor={(name) => setAuthor(name)}
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
        title={t("Tweets")}
        subtitle={latest ? relativeTime(latest, language) : undefined}
      />
      {ready && (
        <Toolbar
          start={
            <input
              className="xc-input feeds-author"
              value={author}
              onChange={(e) => setAuthor(e.target.value)}
              placeholder={t("Filter by author")}
              aria-label={t("Filter by author")}
            />
          }
          end={
            <SearchBox
              value={q}
              onChange={setQ}
              placeholder={t("Search tweets")}
              clearLabel={t("Clear")}
            />
          }
        />
      )}
      {body}
    </div>
  );
}

/** 作者那一栏是“名字 @账号”，筛选时只取账号。 */
export function handleOf(author: string): string {
  const m = author.match(/@\w+/);
  return m ? m[0] : author;
}

function TweetRow({
  tweet,
  onAuthor,
}: {
  tweet: FeedsTweet;
  onAuthor: (name: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const name = tweet.author || "X";
  return (
    <div className="feeds-row">
      <div className="feeds-main feeds-tweet">
        <small>
          <button
            className="feeds-author-link"
            title={t("Filter by author")}
            onClick={() => onAuthor(handleOf(name))}
          >
            {name}
          </button>
          {tweet.time && ` · ${relativeTime(tweet.time, language)}`}
        </small>
        <p className="feeds-tweet-text">{tweet.text}</p>
      </div>
      <div className="feeds-side">
        <a
          className="xc-btn ghost small icon"
          href={tweet.url}
          target="_blank"
          rel="noopener noreferrer"
          title={t("Open on X")}
          aria-label={`${t("Open on X")} ${name}`}
        >
          <ExternalLink size={15} />
        </a>
        <SaveLink url={tweet.url} label={name} compact />
      </div>
    </div>
  );
}
