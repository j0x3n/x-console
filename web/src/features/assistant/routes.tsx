import { lazy, Suspense } from "react";
import type { RouteObject } from "react-router";
import { Loading } from "../../components/ui/States";
import "./i18n";
import "./assistant.css";

const AssistantPage = lazy(() => import("./AssistantPage"));

export const routes: RouteObject[] = [
  {
    path: "assistant/*",
    element: (
      <Suspense fallback={<Loading />}>
        <AssistantPage />
      </Suspense>
    ),
    handle: { title: "Assistant" },
  },
];
