import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../../api/client";
import { QueryState } from "./shared";
import "./traffic.css";

type Usage = { upload: number; download: number };
type Account = {
  user: string;
  today: Usage;
  month: Usage;
  latest: null | {
    ts: number;
    active: number;
    upload_bps: number;
    download_bps: number;
  };
};
export type Traffic = { time: number; accounts: Account[] };
export function trafficBytes(n: number) {
  return n >= 1e9 ? `${(n / 1e9).toFixed(2)} GB` : `${(n / 1e6).toFixed(1)} MB`;
}
export default function TodayTrafficCard() {
  const q = useQuery({
    queryKey: ["overview", "traffic"],
    queryFn: async (): Promise<Traffic> => {
      const r = await apiFetch("/dashboard/traffic");
      return r.json();
    },
    refetchInterval: 10000,
  });
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const d = q.data;
  return (
    <div className="traffic-compact">
      <div className="traffic-compact-row traffic-compact-head">
        <span>账号</span>
        <span>今日 / 本月</span>
        <span>↑ / ↓ Mbps</span>
      </div>
      {d.accounts.map((a) => {
        const l = a.latest;
        const fresh = l && Date.now() / 1000 - l.ts < 35;
        return (
          <div className="traffic-compact-row" key={a.user}>
            <strong title={a.user === "vip" ? "不限速" : "上下行各 5 Mbps"}>
              {a.user}
            </strong>
            <span title="上传与下载合计">
              {trafficBytes(a.today.upload + a.today.download)} /{" "}
              {trafficBytes(a.month.upload + a.month.download)}
            </span>
            <span title={fresh ? `${l.active} 条活动隧道` : "暂无近期采样"}>
              {fresh
                ? `${(l.upload_bps / 1e6).toFixed(2)} / ${(l.download_bps / 1e6).toFixed(2)}`
                : "—"}
            </span>
          </div>
        );
      })}
      <a
        className="traffic-compact-footer"
        href="https://audit.xcc.im/"
        target="_blank"
        rel="noopener noreferrer"
      >
        用量为上下行合计 · 完整统计 ↗
      </a>
    </div>
  );
}
