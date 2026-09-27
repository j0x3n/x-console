import { lazy } from "react";
import type { RouteObject } from "react-router";
import { ClipboardPaste } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "../servers/servers.css";
import "../servers/i18n";
import "./i18n";
import "./pc.css";

const PcPage = lazy(() => import("./PcPage"));

registerCommands([
  {
    id: "pc.clipboard",
    title: "发送到电脑剪贴板",
    group: "本机",
    keywords: "clipboard pc copy paste",
    icon: ClipboardPaste,
    run: ({ navigate }) => navigate("/pc"),
  },
]);

export const routes: RouteObject[] = [
  { path: "pc", element: <PcPage />, handle: { title: "This PC" } },
  { path: "pc/:tab", element: <PcPage />, handle: { title: "This PC" } },
];
