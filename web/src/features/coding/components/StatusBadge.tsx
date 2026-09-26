import { useT } from "../../../contexts/LanguageContext";
import { STATUS_LABELS, STATUS_TONES, type TaskStatus } from "../logic";

export default function StatusBadge({ status }: { status: TaskStatus }) {
  const t = useT();
  const tone = STATUS_TONES[status];
  return (
    <span className={`xc-badge ${tone} coding-status coding-status-${status}`}>
      {status === "running" && <span className="coding-pulse" aria-hidden />}
      {t(STATUS_LABELS[status])}
    </span>
  );
}
