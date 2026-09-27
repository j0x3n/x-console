import { lazy, Suspense } from "react";
import type { RouteObject } from "react-router";
import { Loading } from "../../components/ui/States";
import "./i18n";
import "./automations.css";

const AutomationsPage = lazy(() => import("./AutomationsPage"));

export const routes: RouteObject[] = [
  {
    path: "automations/*",
    element: (
      <Suspense fallback={<Loading />}>
        <AutomationsPage />
      </Suspense>
    ),
    handle: { title: "Automations" },
  },
];
