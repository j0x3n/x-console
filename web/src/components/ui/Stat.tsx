import type { ReactNode } from "react";
import { Link } from "react-router";

/*
 * 页面顶部的一排概要卡片，样式来自初版的“今日”。
 * 每张卡片：左上标签，右上角标，大数字，中间可以放迷你图，底部一行说明。
 */

export function StatStrip({
  children,
  label,
  size,
}: {
  children: ReactNode;
  label?: string;
  /** 默认是紧凑的卡片。今日页用 large。 */
  size?: "large";
}) {
  return (
    <section
      className={`xc-stats${size === "large" ? " large" : ""}`}
      aria-label={label}
    >
      {children}
    </section>
  );
}

export type Tone = "ok" | "warn" | "danger" | "info" | "accent";

interface StatCardProps {
  label: string;
  caption?: ReactNode;
  value?: ReactNode;
  unit?: ReactNode;
  foot?: ReactNode;
  tone?: Tone;
  to?: string;
  onClick?: () => void;
  children?: ReactNode;
  className?: string;
}

export function StatCard({
  label,
  caption,
  value,
  unit,
  foot,
  tone,
  to,
  onClick,
  children,
  className = "",
}: StatCardProps) {
  const body = (
    <>
      <div className="xc-stat-top">
        <span>{label}</span>
        {caption != null && <span>{caption}</span>}
      </div>
      {value != null && (
        <div className={`xc-stat-value ${tone ?? ""}`}>
          {value}
          {unit != null && <small>{unit}</small>}
        </div>
      )}
      {children && <div className="xc-stat-visual">{children}</div>}
      {foot != null && <div className="xc-stat-foot">{foot}</div>}
    </>
  );
  const cls = `xc-stat${value == null ? " no-value" : ""} ${className}`;
  if (to)
    return (
      <Link className={cls} to={to}>
        {body}
      </Link>
    );
  if (onClick)
    return (
      <button type="button" className={cls} onClick={onClick}>
        {body}
      </button>
    );
  return <div className={cls}>{body}</div>;
}

/** 圆环进度。value 超过 max 时按满格画。 */
export function Ring({
  value,
  max,
  size = 44,
  stroke = 5,
  tone = "accent",
  children,
}: {
  value: number;
  max: number;
  size?: number;
  stroke?: number;
  tone?: Tone;
  children?: ReactNode;
}) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const ratio = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0;
  return (
    <span className={`xc-ring ${tone}`} style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <circle
          className="track"
          cx={size / 2}
          cy={size / 2}
          r={r}
          strokeWidth={stroke}
        />
        {/* 比例为 0 时不画：圆头的线长度为 0 也会画出一个点 */}
        {ratio > 0 && (
          <circle
            className="bar"
            cx={size / 2}
            cy={size / 2}
            r={r}
            strokeWidth={stroke}
            strokeDasharray={`${c * ratio} ${c}`}
            transform={`rotate(-90 ${size / 2} ${size / 2})`}
          />
        )}
      </svg>
      {children != null && <span className="xc-ring-label">{children}</span>}
    </span>
  );
}

/** 分段条：每段一个颜色，宽度按数值比例。 */
export function Segments({
  parts,
}: {
  parts: { value: number; tone?: Tone | "muted"; label?: string }[];
}) {
  const total = parts.reduce((sum, p) => sum + p.value, 0);
  return (
    <div className="xc-segments" role="img">
      {total === 0 ? (
        <i className="muted" style={{ flex: 1 }} />
      ) : (
        parts
          .filter((p) => p.value > 0)
          .map((p, i) => (
            <i
              key={i}
              className={p.tone ?? "accent"}
              style={{ flex: p.value }}
              title={p.label}
            />
          ))
      )}
    </div>
  );
}

/** 小柱状图，比如最近 7 天的完成数。 */
export function MiniBars({
  values,
  max,
  tone = "accent",
  labels,
}: {
  values: number[];
  max?: number;
  tone?: Tone;
  labels?: string[];
}) {
  const top = max ?? Math.max(1, ...values);
  return (
    <div className="xc-minibars">
      <div className="bars">
        {values.map((v, i) => (
          <i
            key={i}
            className={v > 0 ? tone : "empty"}
            style={{ height: `${Math.max(8, (v / top) * 100)}%` }}
            title={labels?.[i] ? `${labels[i]}: ${v}` : String(v)}
          />
        ))}
      </div>
      {labels && (
        <div className="labels">
          {labels.map((l, i) => (
            <span key={i}>{l}</span>
          ))}
        </div>
      )}
    </div>
  );
}

/** 带标题、数量和右侧操作的分区。 */
export function Section({
  title,
  count,
  aside,
  children,
  className = "",
}: {
  title: ReactNode;
  count?: number;
  aside?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`xc-section ${className}`}>
      <div className="xc-section-head">
        <h2>
          {title}
          {count != null && <span className="xc-count">{count}</span>}
        </h2>
        {aside && <div className="xc-row">{aside}</div>}
      </div>
      {children}
    </section>
  );
}
