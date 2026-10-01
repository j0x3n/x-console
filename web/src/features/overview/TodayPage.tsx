import { MailCard, MonitoringCard } from "./components/ExtraCards";
import { useModules, type ModuleId } from "../../app/modules";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import {
  ChevronDown,
  ChevronUp,
  Eye,
  EyeOff,
  GripVertical,
  Pencil,
  X,
} from "lucide-react";
import { errorMessage } from "../../api/client";
import { useAuthStatus } from "../../api/core";
import PageHeading from "../../components/ui/PageHeading";
import { Section } from "../../components/ui/Stat";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../lib/time";
import { useNow } from "../calendar/hooks";
import { useHosts } from "../servers/api";
import { useDashboardLayout, useSaveLayout } from "./api";
import CardBoundary from "./components/CardBoundary";
import { DecisionsCard, TodosCard, useTodoCount } from "./components/MainCards";
import {
  ActivityCard,
  HabitsCard,
  HomeCard,
  ScheduleCard,
  WeatherStrip,
} from "./components/SideCards";
import { FitnessSummary } from "../habits/FitnessModule";
import { MoreLink } from "./components/shared";
import TodayStats from "./components/TodayStats";
import {
  cardDefs,
  columnCards,
  defaultLayout,
  moveCard,
  shiftCard,
  spreadSide,
  toggleCard,
  type Column,
  type LayoutCard,
  cardModule,
} from "./layout";
import { greetingKey, summaryLine } from "./today";

const cardBodies: Record<string, { body: () => ReactNode; more?: string }> = {
  todos: { body: () => <TodosCard />, more: "/projects" },
  decisions: { body: () => <DecisionsCard />, more: "/coding/tasks" },
  schedule: { body: () => <ScheduleCard />, more: "/calendar" },
  habits: { body: () => <HabitsCard />, more: "/habits" },
  home: { body: () => <HomeCard />, more: "/home" },
  mail: { body: () => <MailCard />, more: "/mail" },
  monitoring: { body: () => <MonitoringCard />, more: "/monitoring" },
  activity: { body: () => <ActivityCard /> },
  fitness: { body: () => <FitnessSummary compact />, more: "/habits" },
};

/** 按内容区宽度决定几列：窄屏一列，常见宽度两列，2K 屏三列，更宽四列。 */
function columnCount(width: number) {
  if (width >= 2300) return 4;
  if (width >= 1500) return 3;
  if (width >= 700) return 2;
  return 1;
}

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.clientWidth);
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width] as const;
}

export default function TodayPage() {
  const t = useT();
  const language = useLanguage();
  // 问候语只看小时，不需要每 30 秒重画整页；时间单独放在 TodayClock 里走。
  const now = new Date();
  const auth = useAuthStatus();
  const todo = useTodoCount();
  const hosts = useHosts("server");
  const layout = useDashboardLayout();
  const save = useSaveLayout();
  const [draft, setDraft] = useState<LayoutCard[] | null>(null);
  // 服务端还没有布局接口时，改动只留在这次打开的页面里。
  const [local, setLocal] = useState<LayoutCard[] | null>(null);
  const editing = draft !== null;
  const cards = draft ?? local ?? layout.data?.cards ?? defaultLayout();
  const unavailable = layout.data?.unavailable ?? false;
  const [gridRef, gridWidth] = useWidth<HTMLDivElement>();
  const columns = columnCount(gridWidth);
  const modules = useModules();
  const shown = (column: Column) =>
    columnCards(cards, column).filter(
      (c) =>
        (editing || c.visible) &&
        modules.has(cardModule(c.id) as ModuleId | null),
    );
  const weather = cards.find((c) => c.id === "weather");

  const servers = hosts.data ?? [];
  const summary = summaryLine(
    {
      // B57：被隐藏的模块不算进概况
      dueToday: modules.has("projects") ? todo.issues : 0,
      reminders: modules.has("reminders") ? todo.reminders : 0,
      serversTotal: modules.has("servers") ? servers.length : 0,
      serversOffline: modules.has("servers")
        ? servers.filter((h) => !h.online).length
        : 0,
      alerts: modules.has("servers")
        ? servers.reduce((sum, h) => sum + h.activeAlerts, 0)
        : 0,
    },
    language,
  );
  const name = auth.data?.username;
  const greeting =
    t(greetingKey(now)) +
    (name ? `${language === "zh" ? "，" : ", "}${name}` : "");

  const finish = () => {
    if (!draft) return;
    if (unavailable) {
      setLocal(draft);
      setDraft(null);
      return;
    }
    save.mutate(draft, {
      onSuccess: () => {
        setDraft(null);
        toast(t("Saved"));
      },
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };

  return (
    <div className="xc-page today-page">
      <PageHeading
        showTitle
        title={greeting}
        subtitle={summary || undefined}
        aside={
          editing ? (
            <>
              <button
                className="xc-btn small ghost"
                onClick={() => setDraft(null)}
              >
                <X size={14} /> {t("Cancel")}
              </button>
              <button
                className="xc-btn small primary"
                disabled={save.isPending}
                onClick={finish}
              >
                {t("Done editing")}
              </button>
            </>
          ) : (
            <button
              className="xc-btn small ghost"
              onClick={() => setDraft(cards)}
            >
              <Pencil size={14} /> {t("Edit layout")}
            </button>
          )
        }
        meta={
          <>
            <TodayClock />
            {weather &&
              (editing ? (
                <button
                  className={`xc-btn small ghost today-weather-toggle${weather.visible ? "" : " is-off"}`}
                  aria-pressed={weather.visible}
                  aria-label={weather.visible ? t("Hide card") : t("Show card")}
                  title={weather.visible ? t("Hide card") : t("Show card")}
                  onClick={() => setDraft(toggleCard(cards, "weather"))}
                >
                  {weather.visible ? <Eye size={14} /> : <EyeOff size={14} />}
                  {t("Weather")}
                </button>
              ) : (
                weather.visible && <WeatherStrip />
              ))}
          </>
        }
      />
      {editing && (
        <div className={`today-edit-note${unavailable ? " is-warn" : ""}`}>
          {unavailable
            ? t(
                "Saving the layout is not live yet. Changes last until you leave this page.",
              )
            : t(
                "Drag cards to reorder. Hidden cards stay hidden until you show them again.",
              )}
        </div>
      )}
      <TodayStats />
      <div
        ref={gridRef}
        className="today-columns"
        style={{
          gridTemplateColumns:
            columns === 1
              ? "minmax(0, 1fr)"
              : columns === 2
                ? "minmax(0, 1.96fr) minmax(300px, 1fr)"
                : `minmax(0, 1.3fr) repeat(${columns - 1}, minmax(0, 1fr))`,
        }}
      >
        {[
          shown("main"),
          ...(columns === 1
            ? [shown("side")]
            : spreadSide(shown("side"), columns - 1)),
        ].map((list, i) => (
          <div className="today-column" key={i}>
            {list.map((c) => (
              <TodayCard
                key={c.id}
                card={c}
                editing={editing}
                onToggle={() => setDraft(toggleCard(cards, c.id))}
                onShift={(delta) => setDraft(shiftCard(cards, c.id, delta))}
                onDrop={(from) => setDraft(moveCard(cards, from, c.id))}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

function TodayClock() {
  const language = useLanguage();
  const now = useNow(30_000);
  return (
    <time dateTime={now.toISOString()}>
      {formatDate(now, language)} · {formatTime(now, language)}
    </time>
  );
}

function TodayCard({
  card,
  editing,
  onToggle,
  onShift,
  onDrop,
}: {
  card: LayoutCard;
  editing: boolean;
  onToggle: () => void;
  onShift: (delta: -1 | 1) => void;
  onDrop: (fromId: string) => void;
}) {
  const t = useT();
  const [over, setOver] = useState(false);
  const def = cardDefs.find((d) => d.id === card.id);
  const entry = cardBodies[card.id];
  if (!def || !entry) return null;
  const title = t(def.title);

  const aside = editing ? (
    <>
      <button
        className="xc-btn small ghost"
        aria-label={t("Move up")}
        title={t("Move up")}
        onClick={() => onShift(-1)}
      >
        <ChevronUp size={14} />
      </button>
      <button
        className="xc-btn small ghost"
        aria-label={t("Move down")}
        title={t("Move down")}
        onClick={() => onShift(1)}
      >
        <ChevronDown size={14} />
      </button>
      <button
        className="xc-btn small ghost"
        aria-pressed={card.visible}
        aria-label={card.visible ? t("Hide card") : t("Show card")}
        title={card.visible ? t("Hide card") : t("Show card")}
        onClick={onToggle}
      >
        {card.visible ? <Eye size={14} /> : <EyeOff size={14} />}
      </button>
    </>
  ) : entry.more ? (
    <MoreLink to={entry.more} />
  ) : undefined;

  return (
    <div
      className={`today-card${editing ? " is-editing" : ""}${editing && !card.visible ? " is-hidden" : ""}${over ? " is-over" : ""}`}
      draggable={editing}
      onDragStart={(e) => {
        e.dataTransfer.setData("text/x-today-card", card.id);
        e.dataTransfer.effectAllowed = "move";
      }}
      onDragOver={(e) => {
        if (!editing || !e.dataTransfer.types.includes("text/x-today-card"))
          return;
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        setOver(false);
        const from = e.dataTransfer.getData("text/x-today-card");
        if (from) onDrop(from);
      }}
    >
      <Section
        title={
          <>
            {editing && <GripVertical size={14} className="today-grip" />}
            {title}
          </>
        }
        aside={aside}
      >
        {editing ? null : (
          <CardBoundary title={title} retryLabel={t("Retry")}>
            {entry.body()}
          </CardBoundary>
        )}
      </Section>
    </div>
  );
}
