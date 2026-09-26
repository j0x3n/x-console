import type { RouteObject } from "react-router";
import { House, Settings } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./home.css";
import HomePage from "./HomePage";

registerCommands([
  {
    id: "home.open",
    title: "打开智能家居",
    group: "智能家居",
    keywords: "home assistant ha 灯 开关",
    icon: House,
    run: ({ navigate }) => navigate("/home"),
  },
  {
    id: "home.settings",
    title: "Home Assistant 设置",
    group: "智能家居",
    keywords: "home assistant ha token",
    icon: Settings,
    run: ({ navigate }) => navigate("/settings/homeassistant"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "home/*",
    element: <HomePage />,
    handle: { title: "Smart home" },
  },
];
