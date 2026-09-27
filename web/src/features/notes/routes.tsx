import type { RouteObject } from "react-router";
import { NotebookPen, Search, StickyNote } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./notes.css";
import NotesPage from "./NotesPage";
import { useQuickNote } from "./QuickNote";

// 模块入口：路由和命令面板命令。
registerCommands([
  {
    id: "notes.new",
    title: "新建笔记",
    group: "备忘",
    keywords: "new note memo",
    icon: NotebookPen,
    run: ({ navigate }) => navigate("/notes?new=1"),
  },
  {
    id: "notes.quick",
    title: "快速记录",
    group: "备忘",
    keywords: "quick note capture memo",
    icon: StickyNote,
    run: () => useQuickNote.getState().setOpen(true),
  },
  {
    id: "notes.search",
    title: "搜索笔记",
    group: "备忘",
    keywords: "search notes find",
    icon: Search,
    run: ({ navigate }) => navigate("/notes?focus=search"),
  },
]);

export const routes: RouteObject[] = [
  // 同一个路由，切换笔记时列表不会重新挂载。
  {
    path: "notes/:noteId?",
    element: <NotesPage />,
    handle: { title: "Notes" },
  },
];
