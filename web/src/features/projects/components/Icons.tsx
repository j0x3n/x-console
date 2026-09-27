import {
  CircleCheck,
  CircleDashed,
  CircleDot,
  Circle,
  CircleDotDashed,
  CircleX,
  Minus,
  OctagonAlert,
  SignalHigh,
  SignalLow,
  SignalMedium,
} from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { Label } from "../api";
import { PRIORITY_LABELS, STATUS_LABELS, type IssueStatus } from "../logic";

const statusIcons = {
  backlog: CircleDashed,
  todo: Circle,
  in_progress: CircleDotDashed,
  in_review: CircleDot,
  done: CircleCheck,
  canceled: CircleX,
};

export function StatusIcon({
  status,
  size = 15,
}: {
  status: IssueStatus;
  size?: number;
}) {
  const t = useT();
  const Icon = statusIcons[status];
  return (
    <span
      className={`projects-status ${status}`}
      title={t(STATUS_LABELS[status])}
    >
      <Icon size={size} />
    </span>
  );
}

const priorityIcons = {
  0: Minus,
  1: OctagonAlert,
  2: SignalHigh,
  3: SignalMedium,
  4: SignalLow,
} as const;

export function PriorityIcon({
  priority,
  size = 15,
}: {
  priority: number;
  size?: number;
}) {
  const t = useT();
  const Icon = priorityIcons[priority as keyof typeof priorityIcons] ?? Minus;
  return (
    <span
      className={`projects-priority p${priority}`}
      title={t(PRIORITY_LABELS[priority])}
    >
      <Icon size={size} />
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

export const PROJECT_COLORS = [
  "#cc7752",
  "#e8b454",
  "#5cc98b",
  "#4fb3a9",
  "#70b5f7",
  "#8f86f0",
  "#d57ab4",
  "#85858e",
];
