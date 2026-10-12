import { RefreshCw } from "lucide-react";
import { create } from "zustand";
import { useMetricsInterval } from "../../api/events";
import { useT } from "../../contexts/LanguageContext";

/**
 * 服务器数据的刷新间隔（B26，B187 挪到页头）。存在 localStorage，列表和所有机器的详情共用。
 * 0 是暂停：不再订阅实时数据。
 */
const KEY = "xc.hosts.refresh";
export const refreshChoices = [1000, 5000, 30000, 0] as const;

function read(): number {
  try {
    const raw = localStorage.getItem(KEY);
    const v = Number(raw);
    return raw !== null && (refreshChoices as readonly number[]).includes(v)
      ? v
      : 5000;
  } catch {
    return 5000;
  }
}

export const useHostRefresh = create<{
  ms: number;
  setMs: (ms: number) => void;
}>((set) => ({
  ms: read(),
  setMs: (ms) => {
    set({ ms });
    try {
      localStorage.setItem(KEY, String(ms));
    } catch {
      /* 记不住就算了 */
    }
  },
}));

/** 页头里的刷新间隔下拉框。 */
export function RefreshControl() {
  const t = useT();
  const ms = useHostRefresh((s) => s.ms);
  const setMs = useHostRefresh((s) => s.setMs);
  return (
    <label
      className={`xc-btn servers-refresh${ms === 0 ? " paused" : ""}`}
      title={t("Refresh every")}
    >
      <RefreshCw size={14} />
      <select
        aria-label={t("Refresh every")}
        value={ms}
        onChange={(e) => setMs(Number(e.target.value))}
      >
        {refreshChoices.map((v) => (
          <option key={v} value={v}>
            {v ? `${v / 1000} ${t("sec")}` : t("Paused")}
          </option>
        ))}
      </select>
    </label>
  );
}

/** 让服务端按选的间隔推这台机器的数据。暂停时恢复默认间隔（不订阅也就收不到）。 */
export function HostInterval({ hostId }: { hostId: string }) {
  const ms = useHostRefresh((s) => s.ms);
  useMetricsInterval(hostId, ms);
  return null;
}
