import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { Download } from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { Segmented } from "../../../components/ui/Toolbar";
import { StatCard, StatStrip } from "../../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import {
  useAiUsageRecords,
  useAiUsageSummary,
  usageCsvUrl,
  type AiUsageRecord,
  type AiUsageSummary,
  type AiUsageTotals,
  type UsageFilter,
} from "../api";
import { formatCost, formatTokens } from "./models";
import {
  change,
  formatChange,
  formatRate,
  niceMax,
  rangeDates,
  type UsageRange,
} from "../usage";

/*
 * 设置 → AI 用量（B42）：时间范围、概要数字、按天的输入（命中缓存 / 未命中）、
 * 按模型和来源的两张表、调用明细、导出 CSV。
 */

const SOURCES: Record<string, string> = {
  assistant: "AI assistant",
  host_agent: "Server agent",
  notes: "Notes",
  brief: "Daily brief",
  automation: "Automations",
  coding: "Agent tasks",
  mcp: "Remote AI",
  polish: "AI polish",
  "": "Other",
};

function sourceLabel(source: string) {
  return SOURCES[source] ?? source;
}

/** 关联对象能跳过去的给链接。 */
function refLink(r: AiUsageRecord) {
  if (!r.ref) return undefined;
  if (r.source === "coding") return `/coding/${r.ref}`;
  return undefined;
}

export default function UsagePanel() {
  const t = useT();
  const [range, setRange] = useState<UsageRange>("30d");
  const [custom, setCustom] = useState(() => rangeDates("30d"));
  const [picked, setPicked] = useState<{
    model?: string;
    source?: string;
    status?: "ok" | "error";
  }>({});
  const dates = range === "custom" ? custom : rangeDates(range);
  const byDay = useAiUsageSummary(dates.from, dates.to, "day");
  const byModel = useAiUsageSummary(dates.from, dates.to, "model");
  const bySource = useAiUsageSummary(dates.from, dates.to, "source");
  const filter: UsageFilter = { ...dates, ...picked };

  const ranges: { value: UsageRange; label: string }[] = [
    { value: "today", label: t("Today") },
    { value: "7d", label: t("7 days") },
    { value: "30d", label: t("30 days") },
    { value: "month", label: t("This month") },
    { value: "lastMonth", label: t("Last month") },
    { value: "custom", label: t("Custom") },
  ];

  return (
    <div className="ai-usage">
      <div className="ai-usage-bar">
        <Segmented
          label={t("Time range")}
          value={range}
          options={ranges}
          onChange={(v) => {
            setRange(v);
            setPicked({});
          }}
        />
        {range === "custom" && (
          <div className="ai-usage-dates">
            <input
              type="date"
              className="xc-input"
              aria-label={t("From")}
              value={custom.from}
              max={custom.to}
              onChange={(e) => setCustom({ ...custom, from: e.target.value })}
            />
            <span>–</span>
            <input
              type="date"
              className="xc-input"
              aria-label={t("To")}
              value={custom.to}
              min={custom.from}
              onChange={(e) => setCustom({ ...custom, to: e.target.value })}
            />
          </div>
        )}
        <a
          className="xc-btn small"
          href={usageCsvUrl(filter)}
          download
          title={t("Export the calls below as CSV")}
        >
          <Download size={14} />
          {t("Export CSV")}
        </a>
      </div>

      {byDay.isPending ? (
        <Loading />
      ) : byDay.isError ? (
        <ErrorState error={byDay.error} onRetry={() => byDay.refetch()} />
      ) : (
        <>
          <Totals summary={byDay.data} />
          <section className="xc-card">
            <div className="xc-card-head">
              <h2>{t("Input tokens by day")}</h2>
            </div>
            <DailyChart summary={byDay.data} />
          </section>
          <div className="ai-usage-tables">
            <GroupTable
              title={t("By model")}
              column={t("Model")}
              summary={byModel.data}
              label={(key) => key}
              active={picked.model}
              onPick={(key) =>
                setPicked((p) => ({
                  ...p,
                  model: p.model === key ? undefined : key,
                }))
              }
            />
            <GroupTable
              title={t("By source")}
              column={t("Source")}
              summary={bySource.data}
              label={(key) => t(sourceLabel(key))}
              active={picked.source}
              onPick={(key) =>
                setPicked((p) => ({
                  ...p,
                  source: p.source === key ? undefined : key,
                }))
              }
            />
          </div>
          <Records
            filter={filter}
            picked={picked}
            onClear={(key) => setPicked((p) => ({ ...p, [key]: undefined }))}
            onStatus={(status) => setPicked((p) => ({ ...p, status }))}
          />
        </>
      )}
    </div>
  );
}

function Totals({ summary }: { summary: AiUsageSummary }) {
  const t = useT();
  const cur = summary.total;
  const prev = summary.previous;
  const vs = (a: number, b?: number) => formatChange(change(a, b));
  return (
    <StatStrip label={t("Usage")}>
      <StatCard
        label={t("Calls")}
        value={cur.calls}
        caption={vs(cur.calls, prev?.calls)}
        foot={
          cur.errors
            ? t("{n} failed").replace("{n}", String(cur.errors))
            : t("No failures")
        }
        tone={cur.errors ? "warn" : undefined}
      />
      <StatCard
        label={t("Input tokens")}
        value={formatTokens(cur.inputTokens)}
        caption={vs(cur.inputTokens, prev?.inputTokens)}
        foot={`${t("Cache hit")} ${formatTokens(cur.cachedInputTokens)}`}
      />
      <StatCard
        label={t("Output tokens")}
        value={formatTokens(cur.outputTokens)}
        caption={vs(cur.outputTokens, prev?.outputTokens)}
        foot={
          cur.reasoningTokens
            ? `${t("Reasoning")} ${formatTokens(cur.reasoningTokens)}`
            : undefined
        }
      />
      <StatCard
        label={t("Cache hit rate")}
        value={formatRate(cur.cacheHitRate)}
        foot={
          prev?.cacheHitRate !== undefined
            ? `${t("Previous period")} ${formatRate(prev.cacheHitRate)}`
            : t("Cached input ÷ all input")
        }
        tone="info"
      />
      <StatCard
        label={t("Cost")}
        value={formatCost(cur.cost) || "-"}
        caption={vs(cur.cost ?? 0, prev?.cost)}
        foot={
          cur.costEstimated
            ? t("Part of it priced without cache discount")
            : t("From models.dev prices")
        }
      />
    </StatStrip>
  );
}

/** 按天的输入 token：命中缓存和未命中叠在一起。命中率在悬停提示里。 */
function DailyChart({ summary }: { summary: AiUsageSummary }) {
  const t = useT();
  const language = useLanguage();
  const [hover, setHover] = useState<number | null>(null);
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(640);
  useEffect(() => {
    const el = box.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(([entry]) =>
      setWidth(Math.round(entry.contentRect.width)),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const groups = summary.groups;
  const max = niceMax(Math.max(0, ...groups.map((g) => g.totals.inputTokens)));
  if (summary.total.calls === 0)
    return (
      <div ref={box}>
        <EmptyState title={t("No calls in this period")} />
      </div>
    );
  const H = 160;
  const pad = { top: 8, right: 8, bottom: 22, left: 52 };
  // 按容器的实际宽度画，文字不会被拉伸。太窄时横向滚动。
  const W = Math.max(320, width, groups.length * 10);
  const inner = W - pad.left - pad.right;
  const slot = inner / groups.length;
  const barW = Math.max(3, Math.min(24, slot - 2));
  const y = (v: number) => pad.top + (H - pad.top - pad.bottom) * (1 - v / max);
  const every = Math.ceil(groups.length / 8);
  const shown = hover === null ? null : groups[hover];
  const dayLabel = (key: string) =>
    new Date(key + "T00:00:00").toLocaleDateString(
      language === "zh" ? "zh-CN" : "en",
      { month: "numeric", day: "numeric" },
    );
  return (
    <div className="ai-usage-chart">
      <div className="ai-usage-legend">
        <span>
          <i className="swatch cached" />
          {t("Cache hit")}
        </span>
        <span>
          <i className="swatch miss" />
          {t("Cache miss")}
        </span>
      </div>
      <div
        className="ai-usage-plot"
        ref={box}
        onMouseLeave={() => setHover(null)}
      >
        <svg
          width={W}
          height={H}
          viewBox={`0 0 ${W} ${H}`}
          role="img"
          aria-label={t("Input tokens by day")}
        >
          {[0, 0.25, 0.5, 0.75, 1].map((f) => (
            <g key={f}>
              <line
                className="grid"
                x1={pad.left}
                x2={W - pad.right}
                y1={y(max * f)}
                y2={y(max * f)}
              />
              <text className="tick" x={pad.left - 6} y={y(max * f) + 3}>
                {axisTokens(max * f)}
              </text>
            </g>
          ))}
          {groups.map((g, i) => {
            const x = pad.left + i * slot + (slot - barW) / 2;
            const miss = g.totals.inputTokens - g.totals.cachedInputTokens;
            const yCached = y(g.totals.cachedInputTokens);
            const yTop = y(g.totals.inputTokens);
            const base = y(0);
            return (
              <g
                key={g.key}
                onMouseEnter={() => setHover(i)}
                onClick={() => setHover(i)}
              >
                <rect
                  className="hit"
                  x={pad.left + i * slot}
                  y={pad.top}
                  width={slot}
                  height={H - pad.top - pad.bottom}
                />
                {g.totals.cachedInputTokens > 0 && (
                  <rect
                    className="bar cached"
                    x={x}
                    y={yCached}
                    width={barW}
                    height={Math.max(0, base - yCached)}
                    rx={2}
                  />
                )}
                {miss > 0 && (
                  <rect
                    className="bar miss"
                    x={x}
                    y={yTop}
                    width={barW}
                    height={Math.max(0, yCached - yTop - 2)}
                    rx={2}
                  />
                )}
                {i % every === 0 && (
                  <text
                    className="tick day"
                    x={pad.left + i * slot + slot / 2}
                    y={H - 6}
                  >
                    {dayLabel(g.key)}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
        {shown && (
          <div className="ai-usage-tip">
            <strong>{dayLabel(shown.key)}</strong>
            <span>
              {t("Calls")} {shown.totals.calls}
            </span>
            <span>
              {t("Cache hit")} {formatTokens(shown.totals.cachedInputTokens)}
            </span>
            <span>
              {t("Cache miss")}{" "}
              {formatTokens(
                shown.totals.inputTokens - shown.totals.cachedInputTokens,
              )}
            </span>
            <span>
              {t("Cache hit rate")} {formatRate(shown.totals.cacheHitRate)}
            </span>
            {shown.totals.cost !== undefined && (
              <span>
                {t("Cost")} {formatCost(shown.totals.cost)}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/** 坐标轴上的数字不带小数：200K、1.5M。 */
function axisTokens(n: number) {
  if (n >= 1_000_000) return `${Number((n / 1_000_000).toFixed(1))}M`;
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`;
  return String(Math.round(n));
}

function GroupTable({
  title,
  column,
  summary,
  label,
  active,
  onPick,
}: {
  title: string;
  column: string;
  summary?: AiUsageSummary;
  label: (key: string) => string;
  active?: string;
  onPick: (key: string) => void;
}) {
  const t = useT();
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{title}</h2>
      </div>
      {!summary ? (
        <Loading />
      ) : summary.groups.length === 0 ? (
        <p className="xc-muted">{t("No calls in this period")}</p>
      ) : (
        <div className="xc-table-wrap">
          <table className="xc-table ai-usage-group">
            <thead>
              <tr>
                <th>{column}</th>
                <th>{t("Calls")}</th>
                <th>{t("Input")}</th>
                <th>{t("Output")}</th>
                <th>{t("Cache hit rate")}</th>
                <th>{t("Cost")}</th>
              </tr>
            </thead>
            <tbody>
              {summary.groups.map((g) => (
                <GroupRow
                  key={g.key}
                  name={label(g.key)}
                  sub={g.providerName}
                  totals={g.totals}
                  active={active === g.key}
                  onClick={() => onPick(g.key)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function GroupRow({
  name,
  sub,
  totals,
  active,
  onClick,
}: {
  name: string;
  sub?: string;
  totals: AiUsageTotals;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <tr className={active ? "is-active" : ""} onClick={onClick}>
      <td>
        <button type="button" className="ai-usage-pick" aria-pressed={active}>
          {name}
        </button>
        {sub && <small className="xc-muted">{sub}</small>}
      </td>
      <td>{totals.calls}</td>
      <td>{formatTokens(totals.inputTokens)}</td>
      <td>{formatTokens(totals.outputTokens)}</td>
      <td>{formatRate(totals.cacheHitRate)}</td>
      <td>
        {formatCost(totals.cost) || "-"}
        {totals.costEstimated ? "*" : ""}
      </td>
    </tr>
  );
}

function Records({
  filter,
  picked,
  onClear,
  onStatus,
}: {
  filter: UsageFilter;
  picked: { model?: string; source?: string; status?: "ok" | "error" };
  onClear: (key: "model" | "source") => void;
  onStatus: (status?: "ok" | "error") => void;
}) {
  const t = useT();
  const records = useAiUsageRecords(filter);
  const [open, setOpen] = useState<number | null>(null);
  const rows = useMemo(
    () => records.data?.pages.flatMap((p) => p.items) ?? [],
    [records.data],
  );
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Call details")}</h2>
        <Segmented
          label={t("Status")}
          value={picked.status ?? "all"}
          options={[
            { value: "all", label: t("All") },
            { value: "error", label: t("Failed") },
          ]}
          onChange={(v) => onStatus(v === "all" ? undefined : "error")}
        />
      </div>
      {(picked.model || picked.source) && (
        <div className="ai-usage-chips">
          {picked.model && (
            <button className="xc-btn small" onClick={() => onClear("model")}>
              {picked.model} ×
            </button>
          )}
          {picked.source !== undefined && (
            <button className="xc-btn small" onClick={() => onClear("source")}>
              {t(sourceLabel(picked.source))} ×
            </button>
          )}
        </div>
      )}
      {records.isPending ? (
        <Loading />
      ) : records.isError ? (
        <ErrorState error={records.error} onRetry={() => records.refetch()} />
      ) : rows.length === 0 ? (
        <p className="xc-muted">{t("No calls in this period")}</p>
      ) : (
        <ul className="ai-usage-records">
          {rows.map((r) => (
            <RecordRow
              key={r.id}
              r={r}
              open={open === r.id}
              onToggle={() => setOpen(open === r.id ? null : r.id)}
            />
          ))}
        </ul>
      )}
      {records.hasNextPage && (
        <div className="ai-usage-more">
          <button
            className="xc-btn small"
            disabled={records.isFetchingNextPage}
            onClick={() => records.fetchNextPage()}
          >
            {t("Load more")}
          </button>
        </div>
      )}
    </section>
  );
}

function RecordRow({
  r,
  open,
  onToggle,
}: {
  r: AiUsageRecord;
  open: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const link = refLink(r);
  const at = new Date(r.at).toLocaleString(language === "zh" ? "zh-CN" : "en", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  return (
    <li className={r.status === "error" ? "is-error" : ""}>
      <button type="button" className="ai-usage-row" onClick={onToggle}>
        <time dateTime={r.at}>{at}</time>
        <span className="ai-usage-src">{t(sourceLabel(r.source))}</span>
        <span className="xc-mono ai-usage-model">{r.model}</span>
        <span className="ai-usage-tok">
          {formatTokens(r.inputTokens)} / {formatTokens(r.cachedInputTokens)} /{" "}
          {formatTokens(r.outputTokens)}
        </span>
        <span className="ai-usage-cost">
          {r.status === "error" ? (
            <span className="xc-badge danger">{t("Failed")}</span>
          ) : (
            formatCost(r.cost) || "-"
          )}
        </span>
      </button>
      {open && (
        <dl className="ai-usage-detail">
          <div>
            <dt>{t("Provider")}</dt>
            <dd>{r.providerName}</dd>
          </div>
          <div>
            <dt>{t("Input")}</dt>
            <dd>
              {r.inputTokens}（{t("Cache hit")} {r.cachedInputTokens}
              {r.cacheWriteTokens
                ? `，${t("Cache write")} ${r.cacheWriteTokens}`
                : ""}
              ）
            </dd>
          </div>
          <div>
            <dt>{t("Output")}</dt>
            <dd>
              {r.outputTokens}
              {r.reasoningTokens
                ? `（${t("Reasoning")} ${r.reasoningTokens}）`
                : ""}
            </dd>
          </div>
          <div>
            <dt>{t("Duration")}</dt>
            <dd>{(r.durationMs / 1000).toFixed(1)} s</dd>
          </div>
          {r.ref && (
            <div>
              <dt>{t("Related")}</dt>
              <dd>{link ? <Link to={link}>#{r.ref}</Link> : `#${r.ref}`}</dd>
            </div>
          )}
          {r.error && (
            <div>
              <dt>{t("Error")}</dt>
              <dd className="ai-usage-err">{r.error}</dd>
            </div>
          )}
        </dl>
      )}
    </li>
  );
}
