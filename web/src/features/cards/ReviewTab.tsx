import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { ArrowLeft, CheckCheck, Layers, Play, Plus } from "lucide-react";
import { ApiError, errorMessage } from "../../api/client";
import { EmptyState, Spinner } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate } from "../../lib/time";
import {
  fetchQueue,
  useCardDecks,
  useReviewCard,
  type CardSummary,
} from "./api";
import {
  currentItem,
  durationText,
  intervalText,
  isFinished,
  keyAction,
  rate,
  reveal,
  sessionStats,
  skip,
  startSession,
  type Rating,
  type Session,
} from "./session";
import "./i18n";
import "./cards.css";

const RATINGS: {
  rating: Rating;
  label: string;
  key: "again" | "hard" | "good" | "easy";
}[] = [
  { rating: 1, label: "Again", key: "again" },
  { rating: 2, label: "Hard", key: "hard" },
  { rating: 3, label: "Good", key: "good" },
  { rating: 4, label: "Easy", key: "easy" },
];

/** 复习标签页：没开始时是概况和“开始复习”，开始后一张张走。 */
export default function ReviewTab({
  summary,
  onNew,
}: {
  summary?: CardSummary;
  onNew: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const decks = useCardDecks();
  const [deck, setDeck] = useState("");
  const [session, setSession] = useState<Session | null>(null);
  const [starting, setStarting] = useState(false);

  const start = async () => {
    setStarting(true);
    try {
      const queue = await fetchQueue(deck);
      if (queue.items.length === 0) {
        toast(t("Nothing to review right now"));
        return;
      }
      setSession(startSession(queue.items, Date.now()));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    } finally {
      setStarting(false);
    }
  };

  if (session)
    return (
      <ReviewSession
        session={session}
        setSession={setSession}
        summary={summary}
        onExit={() => setSession(null)}
      />
    );

  if (!summary) return null;
  if (summary.total === 0)
    return (
      <EmptyState title={t("No cards yet")} icon={<Layers size={28} />}>
        <span>
          {t(
            "Select a few words in a note or a saved article and choose “Make a card”. You can also write one yourself.",
          )}
        </span>
        <button className="xc-btn primary small" onClick={onNew}>
          <Plus size={14} /> {t("New card")}
        </button>
      </EmptyState>
    );
  if (summary.due === 0)
    return (
      <EmptyState
        title={t("All done for today")}
        icon={<CheckCheck size={28} />}
      >
        <span>
          {summary.nextDue
            ? `${t("The next card is due on")} ${formatDate(summary.nextDue, language)}.`
            : t("Every card has been reviewed.")}
        </span>
        {summary.newCards > 0 && (
          <span>
            {summary.newCards} {t("new cards are waiting for the next day.")}
          </span>
        )}
      </EmptyState>
    );

  return (
    <div className="xc-card cards-start">
      <div className="cards-start-count">
        <strong>{summary.due}</strong>
        <span>{t("cards to review today")}</span>
      </div>
      <p className="xc-muted">
        {summary.dueReview} {t("to refresh")} · {summary.dueNew}{" "}
        {t("new cards")}
      </p>
      {(decks.data?.length ?? 0) > 1 && (
        <label className="xc-field cards-deck-pick">
          <span>{t("Deck")}</span>
          <select
            className="xc-select"
            value={deck}
            onChange={(e) => setDeck(e.target.value)}
          >
            <option value="">{t("All decks")}</option>
            {decks.data?.map((d) => (
              <option key={d.name} value={d.name}>
                {d.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <button
        className="xc-btn primary"
        disabled={starting}
        onClick={() => void start()}
      >
        {starting ? <Spinner /> : <Play size={15} />} {t("Start review")}
      </button>
      <small className="xc-muted">
        {t("Keys: Space shows the answer, 1 to 4 rate it.")}
      </small>
    </div>
  );
}

/** 键盘事件落在输入框里时不当作快捷键。 */
function typing(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
  );
}

function ReviewSession({
  session,
  setSession,
  summary,
  onExit,
}: {
  session: Session;
  setSession: (fn: (s: Session | null) => Session | null) => void;
  summary?: CardSummary;
  onExit: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const review = useReviewCard();
  const [busy, setBusy] = useState(false);
  // 连按两次键时，第二次在界面更新前就到了，所以用 ref 挡住
  const working = useRef(false);
  const item = currentItem(session);

  const doRate = useCallback(
    async (rating: Rating) => {
      const cur = currentItem(session);
      if (!cur || working.current) return;
      working.current = true;
      setBusy(true);
      try {
        await review.mutateAsync({
          id: cur.card.id,
          rating,
          reps: cur.card.reps,
        });
        setSession((s) => (s ? rate(s, rating, Date.now()) : s));
      } catch (e) {
        toast({ message: errorMessage(e), tone: "error" });
        // 这张卡在别处已经评过或被删了：跳过，不计入
        if (e instanceof ApiError && [404, 409].includes(e.status))
          setSession((s) => (s ? skip(s, Date.now()) : s));
      } finally {
        working.current = false;
        setBusy(false);
      }
    },
    [session, review, setSession],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || typing(e.target)) return;
      // 在按钮上按空格会触发按钮本身的点击，这里不重复处理
      if (e.key === " " && e.target instanceof HTMLButtonElement) return;
      const action = keyAction(e.key, session.revealed);
      if (!action || isFinished(session)) return;
      e.preventDefault();
      if (action.type === "reveal") setSession((s) => (s ? reveal(s) : s));
      else void doRate(action.rating);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [session, doRate, setSession]);

  if (!item) {
    const stats = sessionStats(session);
    return (
      <div className="xc-card cards-summary">
        <h2>
          {stats.total > 0 ? t("Review finished") : t("Nothing was reviewed")}
        </h2>
        {stats.total > 0 && (
          <>
            <p className="cards-summary-main">
              <strong>{stats.total}</strong> {t("cards in")}{" "}
              {durationText(stats.seconds, t)}
            </p>
            <ul className="cards-summary-list">
              {RATINGS.map((r) => (
                <li key={r.rating}>
                  <span>{t(r.label)}</span>
                  <b>{stats.counts[r.rating - 1]}</b>
                </li>
              ))}
            </ul>
            <p className="xc-muted">
              {t("Remembered")} {Math.round(stats.passed * 100)}%
            </p>
          </>
        )}
        {summary?.nextDue && (
          <p className="xc-muted">
            {t("The next card is due on")}{" "}
            {formatDate(summary.nextDue, language)}.
          </p>
        )}
        <button className="xc-btn primary" onClick={onExit}>
          <ArrowLeft size={15} /> {t("Back")}
        </button>
      </div>
    );
  }

  const { card, intervals } = item;
  return (
    <div className="cards-session">
      <div className="cards-progress">
        <span>
          {session.index + 1} / {session.items.length}
        </span>
        <span className="cards-progress-bar" aria-hidden>
          <i
            style={{
              width: `${(session.index / session.items.length) * 100}%`,
            }}
          />
        </span>
        <button className="xc-btn small ghost" onClick={onExit}>
          {t("End review")}
        </button>
      </div>
      <div className="xc-card cards-face" data-testid="card-face">
        <div className="cards-face-meta">
          <span className="xc-badge">{card.deck}</span>
          {card.state === "new" && (
            <span className="xc-badge info">{t("Not seen yet")}</span>
          )}
        </div>
        <p className="cards-text cards-question">{card.question}</p>
        {session.revealed && (
          <>
            <hr />
            <p className="cards-text cards-answer">{card.answer}</p>
            {card.sourceTitle && (
              <p className="cards-source">
                {t("Card source")}{" "}
                {card.sourceLink.startsWith("/") ? (
                  <Link to={card.sourceLink}>{card.sourceTitle}</Link>
                ) : card.sourceLink ? (
                  <a
                    href={card.sourceLink}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {card.sourceTitle}
                  </a>
                ) : (
                  card.sourceTitle
                )}
              </p>
            )}
          </>
        )}
      </div>
      {session.revealed ? (
        <div
          className="cards-rate"
          role="group"
          aria-label={t("Rate this card")}
        >
          {RATINGS.map((r) => (
            <button
              key={r.rating}
              className={`xc-btn cards-rate-${r.key}`}
              disabled={busy}
              onClick={() => void doRate(r.rating)}
            >
              <b>{t(r.label)}</b>
              <small>{intervalText(intervals[r.key], t)}</small>
              <kbd>{r.rating}</kbd>
            </button>
          ))}
        </div>
      ) : (
        <button
          className="xc-btn primary cards-show"
          onClick={() => setSession((s) => (s ? reveal(s) : s))}
        >
          {t("Show answer")} <kbd>{t("Space")}</kbd>
        </button>
      )}
    </div>
  );
}
