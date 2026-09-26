import type { RouteObject } from "react-router";
import AutomationsPage from "./AutomationsPage";
import "./i18n";
import "./automations.css";

export const routes: RouteObject[] = [
  {
    path: "automations/*",
    element: <AutomationsPage />,
    handle: { title: "Automations" },
  },
];
