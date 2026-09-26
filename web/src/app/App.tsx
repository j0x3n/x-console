import { QueryClientProvider } from "@tanstack/react-query";
import { createBrowserRouter, RouterProvider } from "react-router";
import { queryClient } from "../api/query";
import AuthGate from "../auth/AuthGate";
import ComingSoon from "../components/ComingSoon";
import { LanguageContext } from "../contexts/LanguageContext";
import { usePreferenceEffects } from "../hooks/usePreferenceEffects";
import Layout from "./Layout";
import { moduleRoutes } from "./routes";

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
  const language = usePreferenceEffects();
  return (
    <LanguageContext.Provider value={language}>
      <QueryClientProvider client={queryClient}>
        <AuthGate>
          <RouterProvider router={router} />
        </AuthGate>
      </QueryClientProvider>
    </LanguageContext.Provider>
  );
}
