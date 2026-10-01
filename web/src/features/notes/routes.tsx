import { lazy } from "react";
import type { RouteObject } from "react-router";
import { NotebookPen, Plus, Search, SquarePen, StickyNote } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { toast } from "../../hooks/useToast";
import { captureNote } from "./api";
import "./i18n";
import "./notes.css";
import { useQuickNote } from "./QuickNote";
import NotesNavChildren from "./NavChildren";
import { registerNavChildren } from "../../lib/navChildren";
import { registerNavAction } from "../../lib/navBadges";

// 侧边栏“/notes”的二级菜单。
registerNavChildren("/notes", NotesNavChildren);
// 左栏“笔记”这一行的“+”：直接新建一条笔记（B72）。
registerNavAction("/notes", {
  icon: Plus,
  label: "New note",
  run: (navigate) => navigate("/notes?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const NotesPage = lazy(() => import("./NotesPage"));

// 模块入口：路由和命令面板命令。
registerCommands([
  {
    id: "notes.new",
    title: "新建笔记",
    group: "笔记",
    keywords: "new note memo",
    icon: NotebookPen,
    run: ({ navigate }) => navigate("/notes?new=1"),
  },
  {
    id: "notes.quick",
    title: "快速记录",
    group: "笔记",
    keywords: "quick note capture memo",
    icon: StickyNote,
    run: () => useQuickNote.getState().setOpen(true),
  },
  {
    // 在命令面板输入 "> 内容" 回车，直接存成新笔记。
    id: "notes.capture",
    title: "存成笔记",
    group: "笔记",
    keywords: "capture note save quick",
    icon: SquarePen,
    prefix: ">",
    run: async ({ navigate, text }) => {
      if (!text) return;
      const note = await captureNote(text);
      toast({
        message: "已存到便签",
        subtitle: text.length > 40 ? `${text.slice(0, 39)}…` : text,
      });
      if (location.pathname.startsWith("/notes"))
        navigate(`/notes/${note.id}?view=memos`);
    },
  },
  {
    id: "notes.search",
    title: "搜索笔记",
    group: "笔记",
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
