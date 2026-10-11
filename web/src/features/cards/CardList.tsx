import { useEffect, useState } from "react";
import { Link } from "react-router";
import { Layers, Pause, Pencil, Play, Plus, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate } from "../../lib/time";
import {
  useCardDecks,
  useCards,
  useDeleteCard,
  useUpdateCard,
  type Card,
  type CardFilter,
  type CardState,
} from "./api";
import CardDialog from "./CardDialog";
import "./i18n";
import "./cards.css";

const STATE_LABELS: Record<CardState, string> = {
  new: "Not seen yet",
  learning: "In learning",
  review: "Reviewing",
  relearning: "Relearning",
};

/** 卡片标签页：搜索、按牌组和状态筛选，改、暂停、删除。 */
export default function CardList({ onNew }: { onNew: () => void }) {
  const t = useT();
  const language = useLanguage();
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [deck, setDeck] = useState("");
  const [state, setState] = useState<CardState | "">("");
  const [editing, setEditing] = useState<Card | null>(null);
  const decks = useCardDecks();
  const update = useUpdateCard();
  const remove = useDeleteCard();

  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(q.trim()), 250);
    return () => window.clearTimeout(id);
  }, [q]);

  const filter: CardFilter = { q: debounced, deck, state, suspended: null };
  const list = useCards(filter);
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const total = list.data?.pages[0]?.total ?? 0;

  const toggleSuspend = (c: Card) =>
    update.mutate(
      { id: c.id, body: { suspended: !c.suspended } },
      {
        onSuccess: () =>
          toast(c.suspended ? t("Card resumed") : t("Card paused")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );

  const askDelete = async (c: Card) => {
    const ok = await confirmAction({
      title: `${t("Delete this card?")}`,
      description: `${c.question.slice(0, 60)} ${t("The review history of this card is deleted too.")}`,
    });
    if (!ok) return;
    remove.mutate(c.id, {
      onSuccess: () => toast(t("Deleted")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0)
    body = (
      <EmptyState
        title={
          debounced || deck || state ? t("No card matches") : t("No cards yet")
        }
        icon={<Layers size={28} />}
      >
        {!debounced && !deck && !state && (
          <button className="xc-btn primary small" onClick={onNew}>
            <Plus size={14} /> {t("New card")}
          </button>
        )}
      </EmptyState>
    );
  else
    body = (
      <>
        <div className="xc-card cards-list">
          {items.map((c) => (
            <div
              key={c.id}
              className={`cards-row${c.suspended ? " is-paused" : ""}`}
            >
              <button className="cards-row-open" onClick={() => setEditing(c)}>
                <span className="cards-row-main">
                  <strong>{c.question}</strong>
                  <small>{c.answer}</small>
                </span>
                <span className="cards-row-side">
                  <span className="xc-badge">{c.deck}</span>
                  {c.suspended ? (
                    <span className="xc-badge warn">{t("Paused")}</span>
                  ) : (
                    <small className="xc-muted">
                      {c.state === "new"
                        ? t(STATE_LABELS.new)
                        : `${t("Due on")} ${formatDate(c.due, language)}`}
                    </small>
                  )}
                </span>
              </button>
              {c.sourceLink.startsWith("/") && (
                <Link
                  className="xc-btn small ghost"
                  to={c.sourceLink}
                  title={c.sourceTitle}
                >
                  {t("Source")}
                </Link>
              )}
              <MoreMenu
                title={c.question.slice(0, 40)}
                label={t("More")}
                items={[
                  {
                    key: "edit",
                    label: t("Edit"),
                    icon: <Pencil size={14} />,
                    onSelect: () => setEditing(c),
                  },
                  {
                    key: "pause",
                    label: c.suspended ? t("Resume") : t("Pause"),
                    icon: c.suspended ? (
                      <Play size={14} />
                    ) : (
                      <Pause size={14} />
                    ),
                    onSelect: () => toggleSuspend(c),
                  },
                  {
                    key: "delete",
                    label: t("Delete"),
                    icon: <Trash2 size={14} />,
                    danger: true,
                    onSelect: () => void askDelete(c),
                  },
                ]}
              />
            </div>
          ))}
        </div>
        {list.hasNextPage && (
          <button
            className="xc-btn cards-more"
            disabled={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            {t("Load more")} ({items.length} / {total})
          </button>
        )}
      </>
    );

  return (
    <>
      <Toolbar
        start={
          <div className="cards-filters">
            <select
              className="xc-select"
              aria-label={t("Deck")}
              value={deck}
              onChange={(e) => setDeck(e.target.value)}
            >
              <option value="">{t("All decks")}</option>
              {decks.data?.map((d) => (
                <option key={d.name} value={d.name}>
                  {d.name} ({d.total})
                </option>
              ))}
            </select>
            <select
              className="xc-select"
              aria-label={t("Card state")}
              value={state}
              onChange={(e) => setState(e.target.value as CardState | "")}
            >
              <option value="">{t("Any state")}</option>
              {(Object.keys(STATE_LABELS) as CardState[]).map((s) => (
                <option key={s} value={s}>
                  {t(STATE_LABELS[s])}
                </option>
              ))}
            </select>
          </div>
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search cards")}
            clearLabel={t("Clear")}
          />
        }
      />
      {body}
      {editing && (
        <CardDialog card={editing} onClose={() => setEditing(null)} />
      )}
    </>
  );
}
