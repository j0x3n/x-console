import { useQuery } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/codingroi";

/* AI 编码投入产出对账（B129）：每个编码任务的费用、运行时间、审查时间和结果。 */
export const codingroiApi = createApi<paths>();

type S = components["schemas"];
export type RoiReport = S["CodingROIReport"];
export type RoiGroup = S["CodingROIGroup"];
export type RoiTask = S["CodingROITask"];
export type RoiOutcome = S["CodingROIOutcome"];
export type RoiPeriod = "week" | "month";
export type RoiGroupBy = "repo" | "tool" | "agent";

export const roiKeys = {
  all: ["codingroi"] as const,
  report: (period: RoiPeriod, date: string, groupBy: RoiGroupBy) =>
    ["codingroi", "report", period, date, groupBy] as const,
};

// 任务完成、合并时刷新（合并结果由服务端另外记下）
invalidateOn("coding_task.", roiKeys.all);

export function useRoiReport(
  period: RoiPeriod,
  date: string,
  groupBy: RoiGroupBy,
) {
  return useQuery({
    queryKey: roiKeys.report(period, date, groupBy),
    queryFn: () =>
      unwrap(
        codingroiApi.GET("/codingroi/report", {
          params: {
            query: { period, groupBy, ...(date ? { date } : {}) },
          },
        }),
      ),
    retry: false,
  });
}

/** 导出 CSV 的地址，条件和页面一样。 */
export function roiCsvUrl(
  period: RoiPeriod,
  date: string,
  groupBy: RoiGroupBy,
) {
  const q = new URLSearchParams({ period, groupBy });
  if (date) q.set("date", date);
  return `/api/v1/codingroi/report.csv?${q.toString()}`;
}
