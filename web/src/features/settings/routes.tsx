import { lazy } from "react";
import type { RouteObject } from "react-router";
import "./i18n";

const SettingsPage = lazy(() => import("./SettingsPage"));

export const routes: RouteObject[] = [
  {
    path: "settings",
    element: <SettingsPage />,
    handle: { title: "Settings" },
  },
  {
    path: "settings/:tab",
    element: <SettingsPage />,
    handle: { title: "Settings" },
  },
];
