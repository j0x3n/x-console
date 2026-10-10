import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { Bookmark, Check, ExternalLink, Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { errorMessage } from "../../api/client";
import {
  useReadList,
  useUpdateReadItem,
  type ReadItem,
  type ReadView,
} from "./api";
import AddLinkDialog from "./AddLinkDialog";
import { SOURCE_LABELS } from "./format";
import ReadDetail from "./ReadDetail";
import "./i18n";
import "./readlater.css";

const VIEWS: ReadView[] = ["unread", "read", "all"];

/** 输入停下来 250 毫秒后才去服务端搜。 */
function useDebounced(value: string, ms = 250): string {
  const [out, setOut] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setOut(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return out;
}

/** 稍后读（B117）：存下来的链接，按未读、已读、标签和关键词找。 */
export default function ReadLaterPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState("");
  const [openId, setOpenId] = useState<number | null>(null);
  const viewParam = params.get("view");
  const view: ReadView = VIEWS.includes(viewParam as ReadView)
    ? (viewParam as ReadView)
    : "unread";
  const tag = params.get("tag") ?? "";
  const creating = params.get("new") === "1";
  const list = useReadList(view, tag, useDebounced(q.trim()));
  const update = useUpdateReadItem();

  const set = (changes: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [key, value] of Object.entries(changes)) {
          if (value) next.set(key, value);
          else next.delete(key);
        }
        return next;
      },
      { replace: true },
    );

  const counts = list.data?.counts;
  const items = list.data?.items ?? [];
  const tags = list.data?.tags ?? [];
  const filtered = q.trim() !== "" || tag !== "";

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Read later")} icon={<Bookmark size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0 && counts?.all === 0)
    body = (
      <EmptyState title={t("No saved links yet")} icon={<Bookmark size={28} />}>
        <span>
          {t(
            "Save links here. The page text is kept, and the AI writes a short summary and tags.",
          )}
        </span>
        <button
          className="xc-btn primary small"
          onClick={() => set({ new: "1" })}
        >
          <Plus size={14} /> {t("Add link")}
        </button>
      </EmptyState>
    );
  else if (items.length === 0)
    body = (
      <EmptyState
        title={filtered ? t("No link matches") : t("Nothing to read")}
        icon={<Bookmark size={28} />}
      >
        {!filtered && (
          <span>
            {t("Everything is read. Saved links are in the Read and All tabs.")}
          </span>
        )}
      </EmptyState>
    );
  else
    body = (
      <div className="xc-card readlater-list">
        {items.map((item) => (
          <Row
            key={item.id}
            item={item}
            onOpen={() => setOpenId(item.id)}
            onToggle={() =>
              update.mutate(
                { id: item.id, body: { read: !item.read } },
                {
                  onError: (err) =>
                    toast({ message: errorMessage(err), tone: "error" }),
                },
              )
            }
          />
        ))}
      </div>
    );

  const unread = counts?.unread ?? 0;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Read later")}
        subtitle={counts ? `${unread} ${t("Unread items")}` : undefined}
        aside={
          <button
            className="xc-btn primary"
            title={t("Add link")}
            onClick={() => set({ new: "1" })}
          >
            <Plus size={15} /> {t("Add link")}
          </button>
        }
      />
      <StatStrip label={t("Read later")}>
        <StatCard
          label={t("Unread items")}
          value={counts?.unread ?? "–"}
          to="/readlater"
        />
        <StatCard
          label={t("Read items")}
          value={counts?.read ?? "–"}
          to="/readlater?view=read"
        />
        <StatCard
          label={t("All")}
          value={counts?.all ?? "–"}
          to="/readlater?view=all"
        />
        <StatCard
          label={t("Fetch failed")}
          value={counts?.failed ?? "–"}
          tone={(counts?.failed ?? 0) > 0 ? "danger" : undefined}
        />
      </StatStrip>
      <Toolbar
        start={
          <nav className="xc-tabs" aria-label={t("Read later")}>
            {VIEWS.map((v) => (
              <button
                key={v}
                className={v === view ? "active" : ""}
                onClick={() => set({ view: v === "unread" ? null : v })}
              >
                {t(v === "unread" ? "Unread" : v === "read" ? "Read" : "All")}
              </button>
            ))}
          </nav>
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search saved links")}
            clearLabel={t("Clear")}
          />
        }
      />
      {tags.length > 0 && (
        <div className="readlater-tags" role="group" aria-label={t("Tags")}>
          {tags.map((x) => (
            <button
              key={x.tag}
              className={`xc-btn small${x.tag === tag ? " primary" : ""}`}
              aria-pressed={x.tag === tag}
              onClick={() => set({ tag: x.tag === tag ? null : x.tag })}
            >
              {x.tag} <span className="readlater-tag-count">{x.count}</span>
            </button>
          ))}
        </div>
      )}
      {body}
      <AddLinkDialog open={creating} onClose={() => set({ new: null })} />
      <ReadDetail id={openId} onClose={() => setOpenId(null)} />
    </div>
  );
}

function Row({
  item,
  onOpen,
  onToggle,
}: {
  item: ReadItem;
  onOpen: () => void;
  onToggle: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  return (
    <div className={`readlater-row${item.read ? " is-read" : ""}`}>
      <button className="readlater-main" onClick={onOpen}>
        <strong>{item.title}</strong>
        <small>
          {item.site} · {relativeTime(item.createdAt, language)} ·{" "}
          {t(SOURCE_LABELS[item.source])}
        </small>
        {item.summary && <span className="readlater-clip">{item.summary}</span>}
        {item.status === "queued" && (
          <span className="xc-badge info">{t("Fetching…")}</span>
        )}
        {item.status === "failed" && (
          <span className="xc-badge danger">
            {t("Fetch failed")}
            {item.error && ` · ${item.error}`}
          </span>
        )}
        {item.tags.length > 0 && (
          <span className="readlater-tagline">
            {item.tags.map((tag) => (
              <span key={tag} className="xc-badge">
                {tag}
              </span>
            ))}
          </span>
        )}
      </button>
      <div className="readlater-side">
        <a
          className="xc-btn ghost small icon"
          href={item.url}
          target="_blank"
          rel="noopener noreferrer"
          title={t("Open the original page")}
          aria-label={`${t("Open the original page")} ${item.title}`}
        >
          <ExternalLink size={15} />
        </a>
        <button
          className="xc-btn ghost small icon"
          title={item.read ? t("Mark as unread") : t("Mark as read")}
          aria-label={`${item.read ? t("Mark as unread") : t("Mark as read")} ${item.title}`}
          onClick={onToggle}
        >
          <Check size={15} />
        </button>
      </div>
    </div>
  );
}
