import { useEffect, useState, type FormEvent } from "react";
import { isNotLive, errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { ErrorState, Loading, NotLive } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatBytes } from "../../../lib/time";
import {
  useHostTraffic,
  useSaveTrafficPlan,
  type HostTraffic,
  type TrafficCountMode,
  type TrafficPlan,
} from "../api";
import { cumulativeUsed, daysLeft, niceMax, trafficTitle } from "../lib";

const S1 = "var(--servers-series-1)";
const S2 = "var(--servers-series-2)";

/** 概要卡片里的“本月流量”（B27）。点开看图表和设置。 */
export function TrafficCard({ hostId }: { hostId: string }) {
  const t = useT();
  const traffic = useHostTraffic(hostId, "current");
  const [open, setOpen] = useState(false);
  const d = traffic.data;
  const notLive = traffic.isError && isNotLive(traffic.error);
  const limit = d?.plan.limitBytes ?? 0;
  const ratio = d && limit > 0 ? d.usedBytes / limit : 0;
  const tone =
    d && limit > 0
      ? ratio >= 1
        ? "danger"
        : d.plan.alertPercent > 0 && ratio * 100 >= d.plan.alertPercent
          ? "warn"
          : "ok"
      : "";
  return (
    <>
      <button
        type="button"
        className="servers-stat clickable"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        <span>{t(d ? trafficTitle(d.plan) : "Traffic this month")}</span>
        <strong>
          {d ? formatBytes(d.usedBytes) : "—"}
          {d && limit > 0 && (
            <small className="servers-traffic-limit">
              {" "}
              / {formatBytes(limit)}
            </small>
          )}
        </strong>
        {d ? (
          <small>
            ↓ {formatBytes(d.rx)} ↑ {formatBytes(d.tx)}
          </small>
        ) : (
          <small>{notLive ? t("Not live yet") : " "}</small>
        )}
        {d && limit > 0 && (
          <span
            className={`servers-bar ${tone}`}
            role="progressbar"
            aria-label={t("Traffic used")}
            aria-valuenow={Math.round(ratio * 100)}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <i style={{ width: `${Math.min(100, ratio * 100)}%` }} />
          </span>
        )}
      </button>
      {open && <TrafficDialog hostId={hostId} onClose={() => setOpen(false)} />}
    </>
  );
}

function TrafficDialog({
  hostId,
  onClose,
}: {
  hostId: string;
  onClose: () => void;
}) {
  const t = useT();
  const [cycle, setCycle] = useState<"current" | "previous">("current");
  const traffic = useHostTraffic(hostId, cycle);
  const current = useHostTraffic(hostId, "current");
  return (
    <Dialog open wide onClose={onClose} title={t("Traffic")}>
      {traffic.isPending ? (
        <Loading />
      ) : traffic.isError ? (
        isNotLive(traffic.error) ? (
          <NotLive name={t("Traffic statistics")} />
        ) : (
          <ErrorState error={traffic.error} onRetry={() => traffic.refetch()} />
        )
      ) : (
        <>
          <div
            className="servers-traffic-switch servers-segmented"
            role="tablist"
          >
            {(["current", "previous"] as const).map((c) => (
              <button
                key={c}
                role="tab"
                aria-selected={cycle === c}
                className={cycle === c ? "active" : ""}
                onClick={() => setCycle(c)}
              >
                {t(c === "current" ? "This cycle" : "Last cycle")}
              </button>
            ))}
          </div>
          <TrafficSummary data={traffic.data} current={cycle === "current"} />
          <DailyChart data={traffic.data} />
          <CumulativeChart data={traffic.data} />
        </>
      )}
      {current.data && (
        <PlanForm hostId={hostId} plan={current.data.plan} onSaved={onClose} />
      )}
    </Dialog>
  );
}

function TrafficSummary({
  data,
  current,
}: {
  data: HostTraffic;
  current: boolean;
}) {
  const t = useT();
  const limit = data.plan.limitBytes;
  return (
    <dl className="servers-traffic-summary">
      <div>
        <dt>{t("Cycle")}</dt>
        <dd>
          {data.cycleStart} ~ {data.cycleEnd}
        </dd>
      </div>
      {current && (
        <div>
          <dt>{t("Days left")}</dt>
          <dd>{daysLeft(data.cycleEnd)}</dd>
        </div>
      )}
      <div>
        <dt>{t("Used")}</dt>
        <dd>
          {formatBytes(data.usedBytes)}
          {limit > 0 && ` / ${formatBytes(limit)}`}
        </dd>
      </div>
      <div>
        <dt>{t("In / out")}</dt>
        <dd>
          ↓ {formatBytes(data.rx)} ↑ {formatBytes(data.tx)}
        </dd>
      </div>
      {current && data.projectedBytes !== undefined && (
        <div>
          <dt>{t("Projected by the end")}</dt>
          <dd
            className={limit > 0 && data.projectedBytes > limit ? "over" : ""}
          >
            {formatBytes(data.projectedBytes)}
          </dd>
        </div>
      )}
      {data.estimated && (
        <p className="xc-muted servers-traffic-estimated">
          {t(
            "Part of the data is estimated from speed. Update the agent for exact numbers.",
          )}
        </p>
      )}
    </dl>
  );
}

const W = 640;
const H = 150;
const PAD = { top: 10, right: 8, bottom: 20, left: 56 };

function shortDay(day: string) {
  const [, m, d] = day.split("-");
  return `${Number(m)}/${Number(d)}`;
}

/** 每天的流入流出，堆叠柱。 */
function DailyChart({ data }: { data: HostTraffic }) {
  const t = useT();
  const [hover, setHover] = useState<number | null>(null);
  const days = data.days;
  if (days.length === 0) return null;
  const max = niceBytes(Math.max(...days.map((d) => d.rx + d.tx), 1));
  const iw = W - PAD.left - PAD.right;
  const ih = H - PAD.top - PAD.bottom;
  const slot = iw / days.length;
  const bw = Math.max(2, Math.min(18, slot - 2));
  const y = (v: number) => PAD.top + ih - (v / max) * ih;
  const hd = hover !== null ? days[hover] : null;
  return (
    <figure className="servers-traffic-chart">
      <figcaption>
        <span>{t("Daily traffic")}</span>
        <span className="servers-legend">
          <i style={{ background: S1 }} /> {t("In")}
          <i style={{ background: S2 }} /> {t("Out")}
        </span>
      </figcaption>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={t("Daily traffic")}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line
              className="servers-gridline"
              x1={PAD.left}
              x2={W - PAD.right}
              y1={y(max * f)}
              y2={y(max * f)}
            />
            <text
              className="servers-axis"
              x={PAD.left - 6}
              y={y(max * f) + 4}
              textAnchor="end"
            >
              {formatBytes(max * f)}
            </text>
          </g>
        ))}
        {days.map((d, i) => {
          const x = PAD.left + i * slot + (slot - bw) / 2;
          const rxTop = y(d.rx);
          const txTop = y(d.rx + d.tx);
          return (
            <g
              key={d.day}
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
            >
              <rect
                x={PAD.left + i * slot}
                y={PAD.top}
                width={slot}
                height={ih}
                fill="transparent"
              />
              {d.rx > 0 && (
                <rect
                  x={x}
                  y={rxTop}
                  width={bw}
                  height={Math.max(0, y(0) - rxTop)}
                  fill={S1}
                />
              )}
              {d.tx > 0 && (
                <rect
                  x={x}
                  y={txTop}
                  width={bw}
                  height={Math.max(0, rxTop - txTop - (d.rx > 0 ? 2 : 0))}
                  rx={2}
                  fill={S2}
                />
              )}
            </g>
          );
        })}
        <text className="servers-axis" x={PAD.left} y={H - 4}>
          {shortDay(days[0].day)}
        </text>
        <text
          className="servers-axis"
          x={W - PAD.right}
          y={H - 4}
          textAnchor="end"
        >
          {shortDay(days[days.length - 1].day)}
        </text>
      </svg>
      {hd && (
        <div className="servers-traffic-tip" role="status">
          {hd.day} · ↓ {formatBytes(hd.rx)} ↑ {formatBytes(hd.tx)}
        </div>
      )}
    </figure>
  );
}

/** 累计用量折线，有上限时画一条虚线。 */
function CumulativeChart({ data }: { data: HostTraffic }) {
  const t = useT();
  const [hover, setHover] = useState<number | null>(null);
  const days = data.days;
  if (days.length === 0) return null;
  const values = cumulativeUsed(days, data.plan.countMode);
  const limit = data.plan.limitBytes;
  const max = niceBytes(Math.max(values[values.length - 1] ?? 0, limit, 1));
  const iw = W - PAD.left - PAD.right;
  const ih = H - PAD.top - PAD.bottom;
  const x = (i: number) =>
    PAD.left + (days.length === 1 ? iw / 2 : (i / (days.length - 1)) * iw);
  const y = (v: number) => PAD.top + ih - (v / max) * ih;
  const path = values
    .map((v, i) => `${i ? "L" : "M"}${x(i)},${y(v)}`)
    .join(" ");
  return (
    <figure className="servers-traffic-chart">
      <figcaption>
        <span>{t("Used so far")}</span>
        {limit > 0 && (
          <span className="servers-legend">
            <i className="dashed" /> {t("Limit")} {formatBytes(limit)}
          </span>
        )}
      </figcaption>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={t("Used so far")}
        onMouseLeave={() => setHover(null)}
        onMouseMove={(e) => {
          const box = e.currentTarget.getBoundingClientRect();
          const px = ((e.clientX - box.left) / box.width) * W;
          const i = Math.round(((px - PAD.left) / iw) * (days.length - 1));
          setHover(Math.max(0, Math.min(days.length - 1, i)));
        }}
      >
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line
              className="servers-gridline"
              x1={PAD.left}
              x2={W - PAD.right}
              y1={y(max * f)}
              y2={y(max * f)}
            />
            <text
              className="servers-axis"
              x={PAD.left - 6}
              y={y(max * f) + 4}
              textAnchor="end"
            >
              {formatBytes(max * f)}
            </text>
          </g>
        ))}
        {limit > 0 && (
          <line
            className="servers-limit"
            x1={PAD.left}
            x2={W - PAD.right}
            y1={y(limit)}
            y2={y(limit)}
          />
        )}
        <path d={path} fill="none" stroke={S1} strokeWidth={2} />
        {hover !== null && (
          <>
            <line
              className="servers-crosshair"
              x1={x(hover)}
              x2={x(hover)}
              y1={PAD.top}
              y2={PAD.top + ih}
            />
            <circle
              cx={x(hover)}
              cy={y(values[hover])}
              r={4}
              fill={S1}
              className="servers-dot"
            />
          </>
        )}
        <text className="servers-axis" x={PAD.left} y={H - 4}>
          {shortDay(days[0].day)}
        </text>
        <text
          className="servers-axis"
          x={W - PAD.right}
          y={H - 4}
          textAnchor="end"
        >
          {shortDay(days[days.length - 1].day)}
        </text>
      </svg>
      {hover !== null && (
        <div className="servers-traffic-tip" role="status">
          {days[hover].day} · {t("Used")} {formatBytes(values[hover])}
        </div>
      )}
    </figure>
  );
}

/** 纵轴上限按 1024 的整数倍取整。 */
function niceBytes(v: number): number {
  let unit = 1;
  while (v / unit >= 1024 && unit < 1024 ** 4) unit *= 1024;
  return niceMax(v / unit, 1) * unit;
}

const GB = 1024 ** 3;

function PlanForm({
  hostId,
  plan,
  onSaved,
}: {
  hostId: string;
  plan: TrafficPlan;
  onSaved: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const save = useSaveTrafficPlan(hostId);
  const [form, setForm] = useState(plan);
  const [unit, setUnit] = useState<"GB" | "TB">(
    plan.limitBytes >= 1024 * GB ? "TB" : "GB",
  );
  const [limit, setLimit] = useState("");
  useEffect(() => {
    setForm(plan);
    const u = plan.limitBytes >= 1024 * GB ? "TB" : "GB";
    setUnit(u);
    setLimit(
      plan.limitBytes
        ? String(+(plan.limitBytes / (u === "TB" ? 1024 * GB : GB)).toFixed(2))
        : "",
    );
  }, [plan]);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const n = limit.trim() === "" ? 0 : Number(limit);
    if (!Number.isFinite(n) || n < 0)
      return toast({
        message: t("The limit should be a number"),
        tone: "error",
      });
    save.mutate(
      {
        ...form,
        limitBytes: Math.round(n * (unit === "TB" ? 1024 * GB : GB)),
      },
      {
        onSuccess: () => {
          toast(t("Saved"));
          onSaved();
        },
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };
  const modes: TrafficCountMode[] = ["both", "out", "in", "max"];
  const modeLabels: Record<TrafficCountMode, string> = {
    both: "In + out",
    out: "Out only",
    in: "In only",
    max: "The larger one",
  };
  return (
    <form className="servers-traffic-plan" onSubmit={submit}>
      <h3>{t("Traffic settings")}</h3>
      <div className="servers-traffic-grid">
        <label className="xc-field">
          <span>{t("Starts on day")}</span>
          <select
            className="xc-select"
            value={form.startDay}
            onChange={(e) =>
              setForm({ ...form, startDay: Number(e.target.value) })
            }
          >
            {Array.from({ length: 31 }, (_, i) => i + 1).map((d) => (
              <option key={d} value={d}>
                {language === "zh" ? `每月 ${d} 号` : `Day ${d}`}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-field">
          <span>{t("Cycle length")}</span>
          <select
            className="xc-select"
            value={form.periodMonths}
            onChange={(e) =>
              setForm({
                ...form,
                periodMonths: Number(
                  e.target.value,
                ) as TrafficPlan["periodMonths"],
              })
            }
          >
            {[1, 3, 6, 12].map((m) => (
              <option key={m} value={m}>
                {m} {t("month(s)")}
              </option>
            ))}
          </select>
        </label>
        <div className="xc-field">
          <span>{t("Limit")}</span>
          <div className="servers-traffic-limit-row">
            <input
              className="xc-input"
              inputMode="decimal"
              aria-label={t("Limit")}
              placeholder={t("No limit")}
              value={limit}
              onChange={(e) => setLimit(e.target.value)}
            />
            <select
              className="xc-select"
              aria-label={t("Unit")}
              value={unit}
              onChange={(e) => setUnit(e.target.value as "GB" | "TB")}
            >
              <option value="GB">GB</option>
              <option value="TB">TB</option>
            </select>
          </div>
        </div>
        <label className="xc-field">
          <span>{t("How usage is counted")}</span>
          <select
            className="xc-select"
            value={form.countMode}
            onChange={(e) =>
              setForm({
                ...form,
                countMode: e.target.value as TrafficCountMode,
              })
            }
          >
            {modes.map((m) => (
              <option key={m} value={m}>
                {t(modeLabels[m])}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-field">
          <span>{t("Remind at")}</span>
          <select
            className="xc-select"
            value={form.alertPercent}
            onChange={(e) =>
              setForm({ ...form, alertPercent: Number(e.target.value) })
            }
          >
            <option value={0}>{t("Do not remind")}</option>
            {[50, 70, 80, 90, 95].map((p) => (
              <option key={p} value={p}>
                {p}%
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="xc-dialog-actions">
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}
