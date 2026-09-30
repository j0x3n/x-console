import { lazy, Suspense } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createBrowserRouter, RouterProvider } from "react-router";
import { queryClient } from "../api/query";
import AuthGate from "../auth/AuthGate";
import ComingSoon from "../components/ComingSoon";
import { LanguageContext } from "../contexts/LanguageContext";
import { usePreferenceEffects } from "../hooks/usePreferenceEffects";
import { useBrowserViewport } from "../hooks/useBrowserViewport";
import Layout from "./Layout";
import { moduleRoutes } from "./routes";
import { Loading } from "../components/ui/States";

// 云盘分享页（B31）不用登录，在登录检查之外渲染。
const SharePage = lazy(() => import("../features/drive/share/SharePage"));
const shareToken =
  typeof location !== "undefined"
    ? /^\/s\/([A-Za-z0-9_-]+)\/?$/.exec(location.pathname)?.[1]
    : undefined;

const router = createBrowserRouter([
  {
    path: "/",
    element: <Layout />,
    children: [
      ...moduleRoutes,
      {
        path: "*",
        element: <ComingSoon title="Not found" />,
        handle: { title: "Not found" },
      },
    ],
  },
]);

export default function App() {
  useBrowserViewport();
  const language = usePreferenceEffects();
  return (
    <LanguageContext.Provider value={language}>
      <QueryClientProvider client={queryClient}>
        {shareToken ? (
          <Suspense fallback={<Loading />}>
            <SharePage token={shareToken} />
          </Suspense>
        ) : (
          <AuthGate>
            <RouterProvider router={router} />
          </AuthGate>
        )}
      </QueryClientProvider>
    </LanguageContext.Provider>
  );
}
