import { useT } from "../../../contexts/LanguageContext";
import type { Monitor } from "../api";
import type { Tone } from "../lib";

/** 监控状态：正常、失败一次、不可用、已暂停、未检查。 */
export function StatusBadge({ monitor, tone }: { monitor: Monitor; tone: Tone }) {
  const t = useT();
  let label = "Not checked yet";
  if (!monitor.enabled) label = "Paused";
  else if (monitor.lastStatus === "down") label = "Down";
  else if (monitor.consecutiveFailures > 0) label = "Failed once";
  else if (monitor.lastStatus === "up") label = "Healthy";
  return <span className={`xc-badge ${tone}`}>{t(label)}</span>;
}
