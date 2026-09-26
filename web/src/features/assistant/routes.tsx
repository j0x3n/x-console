import type { RouteObject } from "react-router";
import AssistantPage from "./AssistantPage";
import "./i18n";
import "./assistant.css";

export const routes: RouteObject[] = [
  {
    path: "assistant/*",
    element: <AssistantPage />,
    handle: { title: "Assistant" },
  },
];
