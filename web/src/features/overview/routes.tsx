import type { RouteObject } from "react-router";
import "./i18n";
import "./overview.css";
import OverviewPage from "./OverviewPage";

export const routes: RouteObject[] = [
  {
    index: true,
    element: <OverviewPage />,
    handle: { title: "Overview" },
  },
];
