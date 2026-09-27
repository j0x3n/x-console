import type { RouteObject } from "react-router";
import { HardDrive, Search, Trash2 } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./drive.css";
import DrivePage from "./DrivePage";

registerCommands([
  {
    id: "drive.open",
    title: "打开云盘",
    group: "云盘",
    keywords: "drive files cloud storage",
    icon: HardDrive,
    run: ({ navigate }) => navigate("/drive"),
  },
  {
    id: "drive.trash",
    title: "云盘回收站",
    group: "云盘",
    keywords: "drive trash deleted",
    icon: Trash2,
    run: ({ navigate }) => navigate("/drive?view=trash"),
  },
  {
    id: "drive.search",
    title: "搜索文件",
    group: "云盘",
    keywords: "drive search files find",
    icon: Search,
    run: ({ navigate }) => navigate("/drive?focus=search"),
  },
]);

export const routes: RouteObject[] = [
  { path: "drive", element: <DrivePage />, handle: { title: "Drive" } },
];
