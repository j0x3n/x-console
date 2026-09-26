import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/dashboard";

export const dashboardApi = createApi<paths>();
export type DashboardCard = components["schemas"]["DashboardCard"];
export type DashboardLayout = components["schemas"]["DashboardLayout"];

const layoutKey = ["dashboard", "layout"] as const;
invalidateOn("dashboard.layout.", layoutKey);

export function useDashboardLayout() {
  return useQuery({
    queryKey: layoutKey,
    queryFn: () => unwrap(dashboardApi.GET("/dashboard/layout")),
  });
}

export function useSaveDashboardLayout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DashboardLayout) =>
      unwrap(dashboardApi.PUT("/dashboard/layout", { body })),
    onSuccess: (layout) => qc.setQueryData(layoutKey, layout),
  });
}
