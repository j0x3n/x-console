import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  BookOpen,
  CalendarCheck,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
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
import {
  useJournalDay,
  useJournalRecent,
  useJournalSearch,
  usePutDiary,
  type JournalHit,
  type JournalItem,
} from "./api";
import {
  chipLabel,
  dayLabel,
  isInternal,
  kindMeta,
  shiftDay,
  summaryNumbers,
  timeLabel,
} from "./format";
import "./i18n";
import "./journal.css";

const DAY = /^\d{4}-\d{2}-\d{2}$/;

/** 输入停下来 300 毫秒后才去服务端搜。 */
function useDebounced(value: string, ms = 300): string {
  const [out, setOut] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setOut(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return out;
}

/** 每日时间线和日记（B118）：当天各模块发生的事，下面写自己的话。 */
export default function JournalPage() {
  const t = useT();
  const language = useLanguage();
  const [params, setParams] = useSearchParams();
  const recent = useJournalRecent(14);
  const [q, setQ] = useState("");
  const query = useDebounced(q.trim());
  const search = useJournalSearch(query);
  const save = usePutDiary();

  const today = recent.data?.days[0]?.day ?? null;
  const dateParam = params.get("date") ?? "";
  const day = DAY.test(dateParam) ? dateParam : today;
  const data = useJournalDay(day);

  // 日记框里的内容。换了日期才从服务端取，改了一半不会被刷新冲掉。
  const [body, setBody] = useState("");
  const loaded = useRef("");
  useEffect(() => {
    if (!data.data || loaded.current === data.data.day) return;
    loaded.current = data.data.day;
    setBody(data.data.diary.body);
  }, [data.data]);
  const dirty =
    !!data.data &&
    loaded.current === data.data.day &&
    body !== data.data.diary.body;

  const saveDiary = async () => {
    if (!day || !dirty) return;
    try {
      await save.mutateAsync({ day, body });
      toast(body.trim() === "" ? t("Diary cleared") : t("Diary saved"));
    } catch (err) {
      toast({ message: errorMessage(err), tone: "error" });
    }
  };

  const go = async (next: string | null) => {
    if (!next) return;
    if (dirty) await saveDiary();
    setQ("");
    setParams(next === today ? {} : { date: next }, { replace: false });
  };

  const counts = data.data?.counts ?? [];
  const nums = summaryNumbers(counts);
  const searching = query !== "";

  let content;
  if (recent.isError && isNotLive(recent.error))
    return (
      <div className="xc-page">
        <PageHeading title={t("Journal")} />
        <NotLive name={t("Journal")} icon={<BookOpen size={28} />} />
      </div>
    );
  if (!day || data.isPending) content = <Loading />;
  else if (data.isError)
    content = <ErrorState error={data.error} onRetry={() => data.refetch()} />;
  else
    content = (
      <>
        <Timeline items={data.data.items} />
        <section className="xc-card journal-diary" aria-label={t("Diary")}>
          <div className="journal-diary-head">
            <strong>{t("Diary")}</strong>
            <span className="xc-muted journal-diary-state">
              {dirty ? t("Not saved yet") : ""}
            </span>
            <button
              className="xc-btn small primary"
              disabled={!dirty || save.isPending}
              onClick={() => void saveDiary()}
            >
              {t("Save")}
            </button>
          </div>
          <div
            onBlur={(e) => {
              // 焦点还在编辑框内（点格式按钮）时不保存
              if (!e.currentTarget.contains(e.relatedTarget as Node | null))
                void saveDiary();
            }}
          >
            <MarkdownEditor
              label={t("Diary text")}
              value={body}
              onChange={setBody}
              placeholder={t("Write about this day. Markdown is supported.")}
              minRows={6}
              onSubmit={() => void saveDiary()}
            />
          </div>
        </section>
      </>
    );

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Journal")}
        subtitle={
          day
            ? `${dayLabel(day, language)}${day === today ? ` · ${t("Today")}` : ""}`
            : undefined
        }
        aside={
          <button
            className="xc-btn small"
            disabled={!today || day === today}
            onClick={() => void go(today)}
          >
            <CalendarCheck size={14} /> {t("Today")}
          </button>
        }
      />
      <StatStrip label={t("Journal")}>
        <StatCard label={t("Code changes")} value={day ? nums.code : "–"} />
        <StatCard
          label={t("Cards and tasks done")}
          value={day ? nums.done : "–"}
        />
        <StatCard
          label={t("Focus time")}
          value={day ? nums.focusMinutes : "–"}
          unit={t("minutes")}
        />
        <StatCard
          label={t("Habits and workouts")}
          value={day ? nums.habits : "–"}
        />
      </StatStrip>
      <Toolbar
        start={
          <div className="journal-nav">
            <button
              className="xc-btn small icon"
              title={t("Previous day")}
              aria-label={t("Previous day")}
              disabled={!day}
              onClick={() => day && void go(shiftDay(day, -1))}
            >
              <ChevronLeft size={15} />
            </button>
            <input
              className="xc-input journal-date"
              type="date"
              aria-label={t("Pick a date")}
              value={day ?? ""}
              max={today ?? undefined}
              onChange={(e) =>
                DAY.test(e.target.value) && void go(e.target.value)
              }
            />
            <button
              className="xc-btn small icon"
              title={t("Next day")}
              aria-label={t("Next day")}
              disabled={!day || !today || day >= today}
              onClick={() => day && void go(shiftDay(day, 1))}
            >
              <ChevronRight size={15} />
            </button>
          </div>
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search the journal")}
            clearLabel={t("Clear")}
          />
        }
      />
      <nav className="journal-days" aria-label={t("Journal days")}>
        {(recent.data?.days ?? []).map((d) => {
          const [date, weekday] = chipLabel(d.day, language);
          return (
            <button
              key={d.day}
              className={`journal-chip${d.day === day ? " on" : ""}`}
              aria-current={d.day === day ? "date" : undefined}
              onClick={() => void go(d.day)}
            >
              <span>{date}</span>
              <small>{weekday}</small>
              <i className={d.diary ? "has-diary" : ""} aria-hidden="true" />
            </button>
          );
        })}
      </nav>
      {searching ? (
        <Results
          loading={search.isFetching && !search.data}
          error={search.isError ? errorMessage(search.error) : ""}
          hits={search.data?.items ?? []}
          language={language}
          onOpen={(d) => void go(d)}
        />
      ) : (
        content
      )}
    </div>
  );
}

function Timeline({ items }: { items: JournalItem[] }) {
  const t = useT();
  if (items.length === 0)
    return (
      <EmptyState
        title={t("Nothing recorded on this day")}
        icon={<BookOpen size={28} />}
      >
        <span>
          {t(
            "Things from the other modules show up here by themselves. You can still write a diary below.",
          )}
        </span>
      </EmptyState>
    );
  return (
    <ol className="xc-card journal-timeline">
      {items.map((item) => {
        const meta = kindMeta(item.kind);
        const Icon = meta.icon;
        const title = item.link ? (
          isInternal(item.link) ? (
            <Link to={item.link}>{item.title}</Link>
          ) : (
            <a href={item.link} target="_blank" rel="noopener noreferrer">
              {item.title}
            </a>
          )
        ) : (
          item.title
        );
        return (
          <li key={item.id} className="journal-item">
            <time className="journal-time">{timeLabel(item)}</time>
            <span className="journal-icon" title={t(meta.label)}>
              <Icon size={16} aria-label={t(meta.label)} />
            </span>
            <span className="journal-main">
              <strong>{title}</strong>
              {item.detail && <small>{item.detail}</small>}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

function Results({
  hits,
  loading,
  error,
  language,
  onOpen,
}: {
  hits: JournalHit[];
  loading: boolean;
  error: string;
  language: string;
  onOpen: (day: string) => void;
}) {
  const t = useT();
  if (loading) return <Loading />;
  if (error) return <p className="xc-error-text">{error}</p>;
  if (hits.length === 0)
    return (
      <EmptyState
        title={t("No result for this search")}
        icon={<BookOpen size={28} />}
      />
    );
  return (
    <ul className="xc-card journal-results" aria-label={t("Search results")}>
      {hits.map((h, i) => {
        const meta = kindMeta(h.kind);
        const Icon = meta.icon;
        return (
          <li key={`${h.day}-${i}`}>
            <button className="journal-hit" onClick={() => onOpen(h.day)}>
              <span className="journal-icon">
                <Icon size={16} aria-label={t(meta.label)} />
              </span>
              <span className="journal-main">
                <strong>{h.kind === "diary" ? t("Diary") : h.title}</strong>
                {h.snippet && <small>{h.snippet}</small>}
              </span>
              <span className="xc-muted journal-hit-day">
                {dayLabel(h.day, language)}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}
