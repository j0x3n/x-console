import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { ArrowDown, Search, X } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import {
  detectLevel,
  matchesLevel,
  splitHighlight,
  type LevelFilter,
  type LogLevel,
} from "./levels";
import "./log.css";

/*
 * 日志查看（B28 容器日志、B29 系统日志、B31 云盘日志文件共用）。
 * - 只渲染看得到的行（每行固定 20px，不换行，横向在框里滚动）。
 * - 在底部时新日志自动滚到底；往上翻了就停下，底部出现“有 N 条新日志”。
 * - 级别、来源（标准输出 / 错误输出）、关键字过滤都在前端做。
 */
export interface LogLine {
  /** 递增的编号，用作 key。 */
  id: number;
  text: string;
  /** 有结构化级别时传；不传就按关键字判断。 */
  level?: LogLevel;
  stream?: "stdout" | "stderr";
  /** ISO 时间，显示成 HH:mm:ss。 */
  time?: string;
  /** 服务名、unit、来源，显示在级别后面。 */
  source?: string;
}

interface Props {
  lines: LogLine[];
  /** 有没有来源信息：true 显示来源筛选；false 不显示。 */
  hasStream?: boolean;
  /** 滚到顶时加载更早的日志。 */
  onReachTop?: () => void;
  loadingOlder?: boolean;
  /** 工具栏右边额外的控件，比如“实时”开关。 */
  toolbarEnd?: ReactNode;
  /** 框的高度，默认 60vh。 */
  height?: string;
  empty?: ReactNode;
  /** 级别由外面控制（系统日志按级别向服务端查询）。不传时在前端过滤。 */
  level?: LevelFilter;
  onLevelChange?: (level: LevelFilter) => void;
}

const ROW = 20;
const OVERSCAN = 30;

const levelLabels: Record<LevelFilter, string> = {
  all: "All levels",
  error: "Errors only",
  warn: "Warnings and above",
  info: "Info and above",
  debug: "Debug and above",
};

function clock(iso?: string) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("zh-CN", { hour12: false });
}

export default function LogViewer({
  lines,
  hasStream,
  onReachTop,
  loadingOlder,
  toolbarEnd,
  height = "60vh",
  empty,
  level: levelProp,
  onLevelChange,
}: Props) {
  const t = useT();
  const [levelState, setLevelState] = useState<LevelFilter>("all");
  const level = levelProp ?? levelState;
  const setLevel = onLevelChange ?? setLevelState;
  const [stream, setStream] = useState<"all" | "stdout" | "stderr">("all");
  const [query, setQuery] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewH, setViewH] = useState(400);
  const [atBottom, setAtBottom] = useState(true);
  const [unseen, setUnseen] = useState(0);
  const lastCount = useRef(0);
  const firstId = useRef<number | undefined>(undefined);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return lines.filter(
      (l) =>
        matchesLevel(l.level ?? detectLevel(l.text), level) &&
        (stream === "all" || l.stream === stream) &&
        (!q || l.text.toLowerCase().includes(q)),
    );
  }, [lines, level, stream, query]);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setViewH(el.clientHeight));
    ro.observe(el);
    setViewH(el.clientHeight);
    return () => ro.disconnect();
  }, []);

  // 新行进来：在底部就跟着滚；往上翻了就记下有几条没看。
  // 前面插入了更早的日志时，保持现在看到的位置不动。
  useLayoutEffect(() => {
    const el = box.current;
    const added = shown.length - lastCount.current;
    const prepended =
      firstId.current !== undefined &&
      shown.length > 0 &&
      shown[0].id !== firstId.current &&
      shown.findIndex((l) => l.id === firstId.current) > 0;
    if (el && prepended) {
      const n = shown.findIndex((l) => l.id === firstId.current);
      el.scrollTop += n * ROW;
    } else if (el && atBottom) {
      el.scrollTop = el.scrollHeight;
    } else if (added > 0) {
      setUnseen((u) => u + added);
    }
    lastCount.current = shown.length;
    firstId.current = shown[0]?.id;
  }, [shown, atBottom]);

  const onScroll = () => {
    const el = box.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < ROW * 2;
    setAtBottom(bottom);
    if (bottom) setUnseen(0);
    if (el.scrollTop < ROW * 3 && onReachTop && !loadingOlder) onReachTop();
  };
  const toBottom = () => {
    const el = box.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    setAtBottom(true);
    setUnseen(0);
  };

  const start = Math.max(0, Math.floor(scrollTop / ROW) - OVERSCAN);
  const end = Math.min(
    shown.length,
    Math.ceil((scrollTop + viewH) / ROW) + OVERSCAN,
  );
  const visible = shown.slice(start, end);

  return (
    <div className="xc-log">
      <div className="xc-log-toolbar">
        <label className="xc-log-search">
          <Search size={13} />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("Search the log")}
            aria-label={t("Search the log")}
          />
          {query && (
            <button aria-label={t("Clear")} onClick={() => setQuery("")}>
              <X size={12} />
            </button>
          )}
        </label>
        <select
          className="xc-select"
          aria-label={t("Level")}
          value={level}
          onChange={(e) => setLevel(e.target.value as LevelFilter)}
        >
          {(Object.keys(levelLabels) as LevelFilter[]).map((k) => (
            <option key={k} value={k}>
              {t(levelLabels[k])}
            </option>
          ))}
        </select>
        {hasStream && (
          <select
            className="xc-select"
            aria-label={t("Output")}
            value={stream}
            onChange={(e) => setStream(e.target.value as typeof stream)}
          >
            <option value="all">{t("All output")}</option>
            <option value="stdout">{t("Standard output")}</option>
            <option value="stderr">{t("Error output")}</option>
          </select>
        )}
        {toolbarEnd}
        <span className="xc-log-count">
          {shown.length === lines.length
            ? lines.length
            : `${shown.length} / ${lines.length}`}{" "}
          {t("lines")}
        </span>
      </div>
      <div
        ref={box}
        className="xc-log-box"
        style={{ height }}
        onScroll={onScroll}
        role="log"
        aria-live="off"
      >
        {loadingOlder && (
          <div className="xc-log-older">{t("Loading earlier lines")}</div>
        )}
        {shown.length === 0 ? (
          <div className="xc-log-empty">{empty ?? t("No log lines")}</div>
        ) : (
          <div style={{ height: shown.length * ROW, position: "relative" }}>
            <div style={{ transform: `translateY(${start * ROW}px)` }}>
              {visible.map((l) => {
                const lv = l.level ?? detectLevel(l.text);
                return (
                  <div
                    key={l.id}
                    className={`xc-log-line lv-${lv}${l.stream === "stderr" ? " stderr" : ""}`}
                    title={
                      l.time ? new Date(l.time).toLocaleString() : undefined
                    }
                  >
                    {l.time && (
                      <span className="xc-log-time">{clock(l.time)}</span>
                    )}
                    {lv !== "other" && (
                      <span className="xc-log-level">{lv}</span>
                    )}
                    {l.source && (
                      <span className="xc-log-source">{l.source}</span>
                    )}
                    <span className="xc-log-text">
                      {splitHighlight(l.text, query).map((part, i) =>
                        part.hit ? <mark key={i}>{part.text}</mark> : part.text,
                      )}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
      {!atBottom && unseen > 0 && (
        <button className="xc-log-jump" onClick={toBottom}>
          <ArrowDown size={13} /> {unseen} {t("new lines, back to bottom")}
        </button>
      )}
    </div>
  );
}
