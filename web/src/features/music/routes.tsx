import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Music, Pause, Play, SkipForward } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavChildren } from "../../lib/navChildren";
import "./i18n";
import "./music.css";
import MusicNavChildren from "./NavChildren";
import { usePlayer } from "./player";

registerNavChildren("/music", MusicNavChildren);

// 页面按需加载（B6）。全局播放器在 app/GlobalPanels.tsx 里。
const MusicPage = lazy(() => import("./MusicPage"));

registerCommands([
  {
    id: "music.open",
    title: "打开音乐",
    group: "音乐",
    keywords: "music songs player 歌曲 播放器",
    icon: Music,
    run: ({ navigate }) => navigate("/music"),
  },
  {
    id: "music.toggle",
    title: "播放或暂停音乐",
    group: "音乐",
    keywords: "music play pause toggle 播放 暂停",
    icon: Play,
    run: () => usePlayer.getState().toggle(),
  },
  {
    id: "music.pause",
    title: "暂停音乐",
    group: "音乐",
    keywords: "music pause stop 暂停",
    icon: Pause,
    run: () => usePlayer.getState().pause(),
  },
  {
    id: "music.next",
    title: "下一首",
    group: "音乐",
    keywords: "music next skip 下一首 切歌",
    icon: SkipForward,
    run: () => usePlayer.getState().next("manual"),
  },
]);

export const routes: RouteObject[] = [
  { path: "music", element: <MusicPage />, handle: { title: "Music" } },
];
