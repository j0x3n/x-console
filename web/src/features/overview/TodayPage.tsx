import { MailCard, MonitoringCard } from "./components/ExtraCards";
import { useModules, type ModuleId } from "../../app/modules";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router";
import {
  ChevronDown,
  ChevronUp,
  Eye,
  EyeOff,
  GripVertical,
  Newspaper,
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
import QuoteLine from "./components/QuoteLine";
import { usePreferencesStore } from "../../stores/preferences-store";
import {
  arrange,
  cardDefs,
  defaultLayout,
  placeCard,
  shiftInColumn,
  toggleCard,
  type LayoutCard,
  cardModule,
} from "./layout";
import { greetingKey, summaryLine } from "./today";
import TodayNetworkCard from "../router/TodayNetworkCard";
import TodayQuotasCard from "../quotas/TodayQuotasCard";
import { useQuotaAccounts } from "../quotas/api";
import TodayMusicCard from "../music/TodayMusicCard";
import { usePlayer } from "../music/player";
import TodayScreenTimeCard from "../screentime/TodayScreenTimeCard";
import { useScreenSummary } from "../screentime/api";

const cardBodies: Record<string, { body: () => ReactNode; more?: string }> = {
  todos: { body: () => <TodosCard />, more: "/projects" },
  decisions: { body: () => <DecisionsCard />, more: "/coding/tasks" },
  schedule: { body: () => <ScheduleCard />, more: "/calendar" },
  habits: { body: () => <HabitsCard />, more: "/habits" },
  home: { body: () => <HomeCard />, more: "/home" },
  network: { body: () => <TodayNetworkCard />, more: "/router" }, // B65
  quotas: { body: () => <TodayQuotasCard />, more: "/quotas" }, // B111
  screentime: { body: () => <TodayScreenTimeCard />, more: "/screentime" }, // B116
  music: { body: () => <TodayMusicCard />, more: "/music" }, // B150
  mail: { body: () => <MailCard />, more: "/mail" },
  monitoring: { body: () => <MonitoringCard />, more: "/monitoring" },
  activity: { body: () => <ActivityCard /> },
  fitness: { body: () => <FitnessSummary compact />, more: "/habits" },
};

const DRAG_TYPE = "text/x-today-card";

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
  // B111：没有额度账号，或接口还没上线时，“AI 额度”卡片不出现
  const quotas = useQuotaAccounts();
  const hasQuotas = (quotas.data?.length ?? 0) > 0;
  // B116：今天还没有电脑时间记录时，“电脑时间”卡片不出现
  const screen = useScreenSummary("day", "", "");
  const hasScreen = (screen.data?.minutes ?? 0) > 0;
  // B150：播放条里没有歌时，“正在播放”卡片不出现
  const hasTrack = usePlayer((s) => s.currentId != null);
  const include = (c: LayoutCard) =>
    (editing || c.visible) &&
    modules.has(cardModule(c.id) as ModuleId | null) &&
    (c.id !== "quotas" || editing || hasQuotas) &&
    (c.id !== "screentime" || editing || hasScreen) &&
    (c.id !== "music" || editing || hasTrack);
  // B88：每张卡片可以放在任意一列，拖到别的列或列的空白处
  const grid = arrange(cards, columns, include);
  const [dragOverColumn, setDragOverColumn] = useState<number | null>(null);
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
  // B88：设置里改的称呼优先，没有时用登录名
  const nickname = usePreferencesStore((st) => st.nickname);
  const name = nickname || auth.data?.username;
  const greeting =
    t(greetingKey(now)) +
    (name ? `${language === "zh" ? "，" : ", "}${name}` : "");

  const weatherToggle = weather && (
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
  );

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
        title={
          <>
            <span className="today-greeting">{greeting}</span>
            {/* B89：问候语后面的每日一句 */}
            <QuoteLine />
          </>
        }
        subtitle={summary || undefined}
        aside={
          editing ? (
            <>
              {weatherToggle}
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
            <>
              {/* B88：时间和天气挪到顶栏，在“早报”左边 */}
              <span className="today-top-info">
                <TodayClock />
                {weather?.visible && <WeatherStrip />}
              </span>
              <Link className="xc-btn small ghost" to="/calendar/briefs">
                <Newspaper size={14} /> {t("Daily brief")}
              </Link>
              <button
                className="xc-btn small ghost"
                onClick={() => setDraft(cards)}
              >
                <Pencil size={14} /> {t("Edit layout")}
              </button>
            </>
          )
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
        {grid.map((list, i) => (
          <div
            className={`today-column${editing ? " is-editing" : ""}${dragOverColumn === i ? " is-over" : ""}`}
            key={i}
            onDragOver={(e) => {
              if (!editing || !e.dataTransfer.types.includes(DRAG_TYPE)) return;
              e.preventDefault();
              setDragOverColumn(i);
            }}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null))
                setDragOverColumn(null);
            }}
            onDrop={(e) => {
              // 放在卡片上时由卡片处理；放在列的空白处时放到这一列最后
              setDragOverColumn(null);
              const from = e.dataTransfer.getData(DRAG_TYPE);
              if (from && !e.defaultPrevented)
                setDraft(placeCard(cards, from, i, null, columns));
            }}
          >
            {list.map((c) => (
              <TodayCard
                key={c.id}
                card={c}
                editing={editing}
                onToggle={() => setDraft(toggleCard(cards, c.id))}
                onShift={(delta) =>
                  setDraft(shiftInColumn(cards, c.id, delta, columns, include))
                }
                onDrop={(from) =>
                  setDraft(placeCard(cards, from, i, c.id, columns))
                }
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
    <time className="today-clock" dateTime={now.toISOString()}>
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
        e.dataTransfer.setData(DRAG_TYPE, card.id);
        e.dataTransfer.effectAllowed = "move";
      }}
      onDragOver={(e) => {
        if (!editing || !e.dataTransfer.types.includes(DRAG_TYPE)) return;
        e.preventDefault();
        e.stopPropagation();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        setOver(false);
        const from = e.dataTransfer.getData(DRAG_TYPE);
        if (!from) return;
        // 告诉外面的列：已经放好了，不要再放到列的最后
        e.preventDefault();
        onDrop(from);
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
