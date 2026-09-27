import type { RouteObject } from "react-router";
import "./i18n";
import "./settings.css";
import SettingsPage from "./SettingsPage";

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
