import type { ReactNode } from "react";
import { useT } from "../../../contexts/LanguageContext";
import type { Label } from "../api";
import { PRIORITY_LABELS, STATUS_LABELS, type IssueStatus } from "../logic";

/*
 * B98：状态和优先级用自己画的图标，颜色在 projects.css 里按类名给。
 * 状态：待规划虚线圆，待办空心圆，进行中半满，待审核四分之三满，
 * 已完成实心圆加勾，已取消实心圆加叉。
 */
const statusShapes: Record<IssueStatus, ReactNode> = {
  backlog: (
    <circle
      cx="8"
      cy="8"
      r="6.2"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeDasharray="2.4 2.2"
    />
  ),
  todo: (
    <circle
      cx="8"
      cy="8"
      r="6.2"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
    />
  ),
  in_progress: (
    <>
      <circle
        cx="8"
        cy="8"
        r="6.2"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
      />
      <path d="M8 4.2a3.8 3.8 0 0 1 0 7.6z" fill="currentColor" />
    </>
  ),
  in_review: (
    <>
      <circle
        cx="8"
        cy="8"
        r="6.2"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
      />
      <path d="M8 4.2a3.8 3.8 0 1 1-3.8 3.8H8z" fill="currentColor" />
    </>
  ),
  done: (
    <>
      <circle cx="8" cy="8" r="7" fill="currentColor" />
      <path
        d="M5 8.2l2 2 4-4.2"
        fill="none"
        stroke="var(--xc-bg)"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </>
  ),
  canceled: (
    <>
      <circle cx="8" cy="8" r="7" fill="currentColor" />
      <path
        d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4"
        stroke="var(--xc-bg)"
        strokeWidth="1.6"
        strokeLinecap="round"
      />
    </>
  ),
};

export function StatusIcon({
  status,
  size = 15,
}: {
  status: IssueStatus;
  size?: number;
}) {
  const t = useT();
  return (
    <span
      className={`projects-status ${status}`}
      title={t(STATUS_LABELS[status])}
    >
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden="true">
        {statusShapes[status] ?? statusShapes.todo}
      </svg>
    </span>
  );
}

/** 三格柱状：低 1 格、中 2 格、高 3 格。紧急是实心方块加“!”，无优先级是三个点。 */
function PriorityShape({ priority }: { priority: number }) {
  if (priority === 1)
    return (
      <>
        <rect
          x="1.5"
          y="1.5"
          width="13"
          height="13"
          rx="3.5"
          fill="currentColor"
        />
        <path
          d="M8 4.6v4.2M8 11.2v.2"
          stroke="var(--xc-bg)"
          strokeWidth="1.8"
          strokeLinecap="round"
        />
      </>
    );
  const filled = { 2: 3, 3: 2, 4: 1 }[priority];
  if (!filled)
    return (
      <path
        d="M3 8h1M7.5 8h1M12 8h1"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    );
  return (
    <>
      {[
        [2, 9, 4],
        [6.5, 6, 7],
        [11, 3, 10],
      ].map(([x, y, h], i) => (
        <rect
          key={x}
          x={x}
          y={y}
          width="3"
          height={h}
          rx="1"
          fill="currentColor"
          opacity={i < filled ? 1 : 0.25}
        />
      ))}
    </>
  );
}

export function PriorityIcon({
  priority,
  size = 15,
}: {
  priority: number;
  size?: number;
}) {
  const t = useT();
  return (
    <span
      className={`projects-priority p${priority}`}
      title={t(PRIORITY_LABELS[priority])}
    >
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden="true">
        <PriorityShape priority={priority} />
      </svg>
    </span>
  );
}

export function LabelChip({ label }: { label: Label }) {
  return (
    <span className="projects-label">
      <i style={label.color ? { background: label.color } : undefined} />
      {label.name}
    </span>
  );
}

/** 项目的小方块图标：颜色 + key 首字母。 */
export function ProjectBadge({
  projectKey,
  color,
}: {
  projectKey: string;
  color: string;
}) {
  return (
    <span
      className="projects-badge"
      style={color ? { background: color } : undefined}
    >
      {projectKey.slice(0, 2)}
    </span>
  );
}

// B98：去掉了原来的褐色，第一个是新建项目的默认颜色
export const PROJECT_COLORS = [
  "#2563eb",
  "#e5793b",
  "#d9a227",
  "#16a34a",
  "#0d9488",
  "#7c3aed",
  "#db2777",
  "#71717a",
];
