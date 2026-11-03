import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../../api/client";
import { QueryState } from "./shared";

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
    <div className="xc-list">
      {d.accounts.map((a) => {
        const l = a.latest;
        const fresh = l && Date.now() / 1000 - l.ts < 35;
        return (
          <div className="today-row" key={a.user}>
            <span className="today-row-main">
              <strong>{a.user}</strong>
              <small>
                今日 ↑ {trafficBytes(a.today.upload)} ↓{" "}
                {trafficBytes(a.today.download)}
              </small>
              <small>
                本月 ↑ {trafficBytes(a.month.upload)} ↓{" "}
                {trafficBytes(a.month.download)}
              </small>
              <small>
                {fresh
                  ? `↑ ${(l.upload_bps / 1e6).toFixed(2)} / ↓ ${(l.download_bps / 1e6).toFixed(2)} Mbps · ${l.active} 条隧道`
                  : "暂无近期采样"}
              </small>
            </span>
            <span className="xc-badge">
              {a.user === "vip" ? "不限速" : "5 Mbps"}
            </span>
          </div>
        );
      })}
      <div className="today-note">
        北京时间 · 每 10 秒采集 · 活动隧道不是在线人数
      </div>
      <a
        className="today-row"
        href="https://audit.xcc.im/traffic/"
        target="_blank"
        rel="noopener noreferrer"
      >
        查看完整流量记录（独立登录）
      </a>
    </div>
  );
}
